package agent

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	acpmcp "github.com/carsonfarmer/go-acp-sdk/mcp"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/mica/pkg/core"
)

// Store is the session store type used by the agent.
type Store = storage.Store[*core.AgentSession]

// Agent implements acp.Agent and optional session lifecycle interfaces.
type Agent struct {
	reg        *core.Registry
	store      storage.Store[*core.AgentSession]
	bc         *agentutil.SessionBroadcaster
	name       string
	tools      []fantasy.AgentTool
	cancellers *agentutil.CancellerMap
}

// Option configures an Agent.
type Option func(*Agent)

// WithName sets the agent name.
func WithName(name string) Option {
	return func(a *Agent) { a.name = name }
}

// WithTools sets the agent's tool set.
func WithTools(tools ...fantasy.AgentTool) Option {
	return func(a *Agent) { a.tools = tools }
}

// New creates a new Agent.
func New(reg *core.Registry, store storage.Store[*core.AgentSession], opts ...Option) *Agent {
	a := &Agent{
		reg:        reg,
		store:      store,
		bc:         agentutil.NewSessionBroadcaster(),
		name:       "mica",
		cancellers: agentutil.NewCancellerMap(),
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

func (a *Agent) Initialize(_ context.Context, req *acp.InitializeRequest) (*acp.InitializeResponse, error) {
	return &acp.InitializeResponse{
		ProtocolVersion: 1,
		AgentCapabilities: &acp.AgentCapabilities{
			LoadSession: true,
			PromptCapabilities: acp.PromptCapabilities{
				Image:           true,
				EmbeddedContext: true,
			},
			SessionCapabilities: acp.SessionCapabilities{
				List:   &acp.SessionListCapabilities{},
				Resume: &acp.SessionResumeCapabilities{},
				Fork:   &acp.SessionForkCapabilities{},
			},
		},
		AgentInfo: &acp.Implementation{Name: a.name, Version: "0.1.0"},
	}, nil
}

func (a *Agent) Authenticate(_ context.Context, _ *acp.AuthenticateRequest) (*acp.AuthenticateResponse, error) {
	return &acp.AuthenticateResponse{}, nil
}

func (a *Agent) NewSession(ctx context.Context, req *acp.NewSessionRequest, client acp.Client) (*acp.NewSessionResponse, error) {
	sid := acp.SessionID(acp.NewUUID())

	sess := &core.AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: sid, CWD: req.CWD},
		Model:       a.reg.Default(),
		Mode:        core.ModeNormal,
	}
	if err := a.store.Set(ctx, sid, sess); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	// Connect MCP servers eagerly.
	mgr := acpmcp.NewManager()
	for i := range req.McpServers {
		if err := mgr.Connect(ctx, &req.McpServers[i]); err != nil {
			var name string
			switch {
			case req.McpServers[i].Stdio != nil:
				name = req.McpServers[i].Stdio.Name
			case req.McpServers[i].HTTP != nil:
				name = req.McpServers[i].HTTP.Name
			case req.McpServers[i].SSE != nil:
				name = req.McpServers[i].SSE.Name
			}
			fmt.Printf("mcp connect %s: %v\n", name, err)
		}
	}

	sess.MCP = mgr

	a.bc.Subscribe(sid, client)

	return &acp.NewSessionResponse{
		SessionID:     sid,
		Modes:         a.getSessionModeState(sess),
		Models:        a.getSessionModelState(sess),
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) LoadSession(ctx context.Context, req *acp.LoadSessionRequest, client acp.Client) (*acp.LoadSessionResponse, error) {
	a.bc.Subscribe(req.SessionID, client)

	sess, _, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	mgr := acpmcp.NewManager()
	for i := range req.McpServers {
		if err := mgr.Connect(ctx, &req.McpServers[i]); err != nil {
			var name string
			switch {
			case req.McpServers[i].Stdio != nil:
				name = req.McpServers[i].Stdio.Name
			case req.McpServers[i].HTTP != nil:
				name = req.McpServers[i].HTTP.Name
			case req.McpServers[i].SSE != nil:
				name = req.McpServers[i].SSE.Name
			}
			fmt.Printf("mcp connect %s: %v\n", name, err)
		}
	}
	sess.MCP = mgr

	events, err := a.store.Load(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	stream := agentutil.NewSessionStream(client, req.SessionID)
	for _, upd := range events {
		if err := stream.SendUpdate(ctx, upd); err != nil {
			return nil, err
		}
	}

	return &acp.LoadSessionResponse{
		Modes:         a.getSessionModeState(sess),
		Models:        a.getSessionModelState(sess),
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) CloseSession(_ context.Context, req *acp.CloseSessionRequest, client acp.Client) (*acp.CloseSessionResponse, error) {
	a.bc.Unsubscribe(req.SessionID, client)
	a.cancellers.Cancel(req.SessionID)
	if sess, _, err := a.store.Get(context.Background(), req.SessionID); err == nil && sess.MCP != nil {
		sess.MCP.Close()
	}
	return &acp.CloseSessionResponse{}, nil
}

func (a *Agent) Cancel(_ context.Context, notif *acp.CancelNotification) error {
	a.cancellers.Cancel(notif.SessionID)
	return nil
}

func (a *Agent) ListSessions(ctx context.Context, _ *acp.ListSessionsRequest) (*acp.ListSessionsResponse, error) {
	sessions, err := a.store.List(ctx)
	if err != nil {
		return nil, err
	}
	return &acp.ListSessionsResponse{Sessions: sessions}, nil
}

func (a *Agent) ResumeSession(ctx context.Context, req *acp.ResumeSessionRequest, client acp.Client) (*acp.ResumeSessionResponse, error) {
	a.bc.Subscribe(req.SessionID, client)
	sess, _, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	mgr := acpmcp.NewManager()
	for i := range req.McpServers {
		if err := mgr.Connect(ctx, &req.McpServers[i]); err != nil {
			var name string
			switch {
			case req.McpServers[i].Stdio != nil:
				name = req.McpServers[i].Stdio.Name
			case req.McpServers[i].HTTP != nil:
				name = req.McpServers[i].HTTP.Name
			case req.McpServers[i].SSE != nil:
				name = req.McpServers[i].SSE.Name
			}
			fmt.Printf("mcp connect %s: %v\n", name, err)
		}
	}
	sess.MCP = mgr
	return &acp.ResumeSessionResponse{
		Modes:         a.getSessionModeState(sess),
		Models:        a.getSessionModelState(sess),
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) ForkSession(ctx context.Context, req *acp.ForkSessionRequest, client acp.Client) (*acp.ForkSessionResponse, error) {
	parent, parentHead, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	sid := acp.SessionID(acp.NewUUID())
	sess := &core.AgentSession{
		SessionInfo:  &acp.SessionInfo{SessionID: sid, CWD: parent.SessionInfo.CWD},
		Model:        parent.Model,
		ThoughtLevel: parent.ThoughtLevel,
		Mode:         parent.Mode,
	}
	if err := a.store.Set(ctx, sid, sess); err != nil {
		return nil, err
	}

	if parentHead != nil {
		if err := a.store.Commit(ctx, sid, *parentHead); err != nil {
			return nil, err
		}
	}

	a.bc.Subscribe(sid, client)

	return &acp.ForkSessionResponse{
		SessionID:     sid,
		Modes:         a.getSessionModeState(sess),
		Models:        a.getSessionModelState(sess),
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

// SetSessionMode implements acp.AgentSessionModeSetter.
func (a *Agent) SetSessionMode(ctx context.Context, req *acp.SetSessionModeRequest, client acp.Client) (*acp.SetSessionModeResponse, error) {
	sess, _, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	m := core.Mode(req.ModeID)
	switch m {
	case core.ModeNormal, core.ModeSafe:
	default:
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "invalid mode")
	}
	sess .Mode = m
	if err := a.store.Set(ctx, req.SessionID, sess); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	a.bc.SessionUpdate(ctx, &acp.SessionNotification{
		SessionID: req.SessionID,
		Update:    acp.UpdateCurrentMode(acp.SessionModeID(m)),
	})
	a.bc.SessionUpdate(ctx, &acp.SessionNotification{
		SessionID: req.SessionID,
		Update:    acp.UpdateConfigOptions(a.getSessionConfigOptions(sess)...),
	})
	return &acp.SetSessionModeResponse{}, nil
}

// SetSessionModel implements acp.AgentSessionModelSetter.
func (a *Agent) SetSessionModel(ctx context.Context, req *acp.SetSessionModelRequest, client acp.Client) (*acp.SetSessionModelResponse, error) {
	sess, _, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	mid := core.FullModelID(req.ModelID)
	if _, ok := a.reg.Config(mid); !ok {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "unknown model")
	}
	sess.Model = mid
	if err := a.store.Set(ctx, req.SessionID, sess); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	a.bc.SessionUpdate(ctx, &acp.SessionNotification{
		SessionID: req.SessionID,
		Update:    acp.UpdateConfigOptions(a.getSessionConfigOptions(sess)...),
	})
	return &acp.SetSessionModelResponse{}, nil
}

// getSessionModeState builds a SessionModeState from the session's current mode.
func (a *Agent) getSessionModeState(sess *core.AgentSession) *acp.SessionModeState {
	cur := acp.SessionModeID(sess.Mode)
	if cur == "" {
		cur = acp.SessionModeID(core.ModeNormal)
	}
	return &acp.SessionModeState{
		AvailableModes: []acp.SessionMode{
			{ID: acp.SessionModeID(core.ModeNormal), Name: "Normal", Description: "Full access — no permission prompts."},
			{ID: acp.SessionModeID(core.ModeSafe), Name: "Safe", Description: "Ask before writes, edits, or commands."},
		},
		CurrentModeID: cur,
	}
}

// getSessionModelState builds a SessionModelState from the registry and session.
func (a *Agent) getSessionModelState(sess *core.AgentSession) *acp.SessionModelState {
	var models []acp.ModelInfo
	for _, pi := range a.reg.Providers() {
		models = append(models, a.reg.Models(pi.ID)...)
	}
	return &acp.SessionModelState{
		CurrentModelID:  acp.ModelID(string(sess.Model)),
		AvailableModels: models,
	}
}

var (
	_ acp.Agent               = (*Agent)(nil)
	_ acp.AgentSessionLoader  = (*Agent)(nil)
	_ acp.AgentSessionCloser  = (*Agent)(nil)
	_ acp.AgentSessionLister  = (*Agent)(nil)
	_ acp.AgentSessionResumer = (*Agent)(nil)
	_ acp.AgentSessionForker  = (*Agent)(nil)
	_ acp.AgentSessionModeSetter  = (*Agent)(nil)
	_ acp.AgentSessionModelSetter = (*Agent)(nil)
)
