package agent

import (
	"context"

	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/mica/pkg/llm"
)

type AgentSession struct {
	*acp.SessionInfo
	Model        llm.FullModelID `json:"model"`
	ThoughtLevel string          `json:"thoughtLevel,omitempty"`
	TotalUsage   acp.Usage       `json:"totalUsage,omitzero"`
	TotalCost    *acp.Cost       `json:"totalCost,omitempty"`
}

// Agent implements acp.Agent and optional session lifecycle interfaces.
type Agent struct {
	reg        *llm.Registry
	store      storage.Store[*AgentSession]
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
func New(reg *llm.Registry, store storage.Store[*AgentSession], opts ...Option) *Agent {
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

	sess := &AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: sid, CWD: req.CWD},
		Model:       a.reg.Default(),
	}
	if err := a.store.Set(ctx, sid, sess); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	a.bc.Subscribe(sid, client)

	return &acp.NewSessionResponse{
		SessionID:     sid,
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) LoadSession(ctx context.Context, req *acp.LoadSessionRequest, client acp.Client) (*acp.LoadSessionResponse, error) {
	a.bc.Subscribe(req.SessionID, client)

	sess, _, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

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
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) CloseSession(_ context.Context, req *acp.CloseSessionRequest, client acp.Client) (*acp.CloseSessionResponse, error) {
	a.bc.Unsubscribe(req.SessionID, client)
	a.cancellers.Cancel(req.SessionID)
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
	return &acp.ResumeSessionResponse{
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

func (a *Agent) ForkSession(ctx context.Context, req *acp.ForkSessionRequest, client acp.Client) (*acp.ForkSessionResponse, error) {
	parent, parentHead, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	sid := acp.SessionID(acp.NewUUID())
	sess := &AgentSession{
		SessionInfo:  &acp.SessionInfo{SessionID: sid, CWD: parent.CWD},
		Model:        parent.Model,
		ThoughtLevel: parent.ThoughtLevel,
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
		ConfigOptions: a.getSessionConfigOptions(sess),
	}, nil
}

var (
	_ acp.Agent               = (*Agent)(nil)
	_ acp.AgentSessionLoader  = (*Agent)(nil)
	_ acp.AgentSessionCloser  = (*Agent)(nil)
	_ acp.AgentSessionLister  = (*Agent)(nil)
	_ acp.AgentSessionResumer = (*Agent)(nil)
	_ acp.AgentSessionForker  = (*Agent)(nil)
)
