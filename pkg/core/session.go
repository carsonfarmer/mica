package core

import (
	"context"
	"strings"

	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
)

type ctxKey string

const (
	ctxClient  ctxKey = "acp-client"
	ctxSession ctxKey = "acp-session"
)

// Mode controls the agent's permission behavior.
type Mode string

const (
	ModeNormal Mode = "normal"
	ModeSafe   Mode = "safe"
)

type AgentSession struct {
	*acp.SessionInfo
	Model        FullModelID          `json:"model"`
	ThoughtLevel string               `json:"thoughtLevel,omitempty"`
	Mode         Mode                 `json:"mode"`
	Permissions  agentutil.Permissions `json:"permissions"`
	Usage        acp.Usage            `json:"usage,omitzero"`
	Cost         *acp.Cost            `json:"cost,omitempty"`
	PromptHooks  []PromptHook         `json:"-"`
}

// PromptHook contributes additional content to the system prompt.
type PromptHook func(*strings.Builder)

// WithClient returns a context carrying the ACP client.
func WithClient(ctx context.Context, c acp.Client) context.Context {
	return context.WithValue(ctx, ctxClient, c)
}

// ClientFrom extracts the ACP client from context.
func ClientFrom(ctx context.Context) acp.Client {
	c, _ := ctx.Value(ctxClient).(acp.Client)
	return c
}

// WithSession returns a context carrying the agent session.
func WithSession(ctx context.Context, s *AgentSession) context.Context {
	return context.WithValue(ctx, ctxSession, s)
}

// SessionFrom extracts the agent session from context.
func SessionFrom(ctx context.Context) *AgentSession {
	s, _ := ctx.Value(ctxSession).(*AgentSession)
	return s
}
