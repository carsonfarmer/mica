// Package agent wires the ACP protocol, the LLM bridge, and storage together
// into a fully functional ACP agent.
package agent

import (
	"context"
	"sync"

	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/mica/pkg/llm"
)

// AgentSession is the persisted session type. It embeds *acp.SessionInfo for
// protocol fields and adds Model for LLM selection.
type AgentSession struct {
	*acp.SessionInfo
	Model string `json:"model"`
}

// Agent implements acp.Agent and optional session lifecycle interfaces.
type Agent struct {
	reg        *llm.Registry
	store      storage.Store[*AgentSession]
	bc         *agentutil.SessionBroadcaster
	name       string
	tools      []fantasy.AgentTool
	cancellers map[acp.SessionID]*agentutil.SessionCanceller
	mu         sync.Mutex
}

// Option configures an Agent.
type Option func(*Agent)

// WithName sets the agent name reported in Initialize responses.
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
		cancellers: make(map[acp.SessionID]*agentutil.SessionCanceller),
	}
	for _, o := range opts {
		o(a)
	}
	return a
}

// ACP interface methods

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

	if err := a.store.Set(ctx, sid, &AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: sid, CWD: req.CWD},
		Model:       a.reg.Default(),
	}); err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	a.bc.Subscribe(sid, client)

	return &acp.NewSessionResponse{SessionID: sid}, nil
}

func (a *Agent) LoadSession(ctx context.Context, req *acp.LoadSessionRequest, client acp.Client) (*acp.LoadSessionResponse, error) {
	a.bc.Subscribe(req.SessionID, client)

	events, err := a.store.Load(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return &acp.LoadSessionResponse{}, nil
	}

	stream := agentutil.NewSessionStream(client, req.SessionID)
	for _, upd := range events {
		if err := stream.SendUpdate(ctx, upd); err != nil {
			return nil, err
		}
	}

	return &acp.LoadSessionResponse{}, nil
}

func (a *Agent) CloseSession(_ context.Context, req *acp.CloseSessionRequest, client acp.Client) (*acp.CloseSessionResponse, error) {
	a.bc.Unsubscribe(req.SessionID, client)

	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.cancellers[req.SessionID]; ok {
		c.End()
		delete(a.cancellers, req.SessionID)
	}

	return &acp.CloseSessionResponse{}, nil
}

func (a *Agent) Cancel(_ context.Context, notif *acp.CancelNotification) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.cancellers[notif.SessionID]; ok {
		c.Cancel()
	}
	return nil
}

func (a *Agent) ListSessions(ctx context.Context, _ *acp.ListSessionsRequest) (*acp.ListSessionsResponse, error) {
	sessions, err := a.store.List(ctx)
	if err != nil {
		return nil, err
	}
	infos := make([]acp.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		infos = append(infos, *s.SessionInfo)
	}
	return &acp.ListSessionsResponse{Sessions: infos}, nil
}

func (a *Agent) ResumeSession(ctx context.Context, req *acp.ResumeSessionRequest, client acp.Client) (*acp.ResumeSessionResponse, error) {
	a.bc.Subscribe(req.SessionID, client)
	if _, err := a.store.Get(ctx, req.SessionID); err != nil {
		return nil, err
	}
	return &acp.ResumeSessionResponse{}, nil
}

func (a *Agent) ForkSession(ctx context.Context, req *acp.ForkSessionRequest, client acp.Client) (*acp.ForkSessionResponse, error) {
	parent, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	head, err := a.store.Head(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}

	sid := acp.SessionID(acp.NewUUID())

	if err := a.store.Set(ctx, sid, &AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: sid, CWD: parent.CWD},
		Model:       parent.Model,
	}); err != nil {
		return nil, err
	}

	if head != nil {
		if err := a.store.Commit(ctx, sid, *head); err != nil {
			return nil, err
		}
	}

	a.bc.Subscribe(sid, client)

	return &acp.ForkSessionResponse{SessionID: sid}, nil
}

// Interface checks.
var (
	_ acp.Agent               = (*Agent)(nil)
	_ acp.AgentSessionLoader  = (*Agent)(nil)
	_ acp.AgentSessionCloser  = (*Agent)(nil)
	_ acp.AgentSessionLister  = (*Agent)(nil)
	_ acp.AgentSessionResumer = (*Agent)(nil)
	_ acp.AgentSessionForker  = (*Agent)(nil)
)
