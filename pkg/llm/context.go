package llm

import (
	"context"
	"fmt"
	"path/filepath"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
)

type ctxKey string

const (
	ctxClient    ctxKey = "acp-client"
	ctxSessionID ctxKey = "acp-session-id"
	ctxStream    ctxKey = "acp-stream"
	ctxCWD       ctxKey = "acp-cwd"
)

// WithClient returns a context carrying the ACP client.
func WithClient(ctx context.Context, c acp.Client) context.Context {
	return context.WithValue(ctx, ctxClient, c)
}

// WithSession returns a context carrying the session ID.
func WithSession(ctx context.Context, sid acp.SessionID) context.Context {
	return context.WithValue(ctx, ctxSessionID, sid)
}

// ClientFrom extracts the ACP client from context.
func ClientFrom(ctx context.Context) acp.Client {
	c, _ := ctx.Value(ctxClient).(acp.Client)
	return c
}

// SessionFrom extracts the session ID from context.
func SessionFrom(ctx context.Context) acp.SessionID {
	s, _ := ctx.Value(ctxSessionID).(acp.SessionID)
	return s
}

// WithCWD returns a context carrying the session working directory.
func WithCWD(ctx context.Context, cwd string) context.Context {
	return context.WithValue(ctx, ctxCWD, cwd)
}

// RelPath returns p relative to the session CWD from context, or p unchanged.
func RelPath(ctx context.Context, p string) string {
	cwd, _ := ctx.Value(ctxCWD).(string)
	if cwd == "" {
		return p
	}
	if r, err := filepath.Rel(cwd, p); err == nil {
		return r
	}
	return p
}

// WithStream returns a context carrying a session stream for broadcasting updates.
func WithStream(ctx context.Context, s *agentutil.SessionStream) context.Context {
	return context.WithValue(ctx, ctxStream, s)
}

// StreamFrom extracts the session stream from context.
func StreamFrom(ctx context.Context) *agentutil.SessionStream {
	s, _ := ctx.Value(ctxStream).(*agentutil.SessionStream)
	return s
}

// WrapCodeBlock wraps content in a markdown code block using the file extension as language.
func WrapCodeBlock(path, content string) string {
	ext := filepath.Ext(path)
	lang := ext
	if len(lang) > 0 {
		lang = lang[1:]
	}
	return fmt.Sprintf("```%s\n%s\n```", lang, content)
}

// ToolResponse builds a tool response whose ClientMetadata carries a
// batch of session updates for the caller to stream and persist.
func ToolResponse(text string, updates ...acp.SessionUpdate) fantasy.ToolResponse {
	return fantasy.WithResponseMetadata(fantasy.NewTextResponse(text), updates)
}

// ToolErrorResponse is like ToolResponse but signals a tool failure.
func ToolErrorResponse(text string, updates ...acp.SessionUpdate) fantasy.ToolResponse {
	return fantasy.WithResponseMetadata(fantasy.NewTextErrorResponse(text), updates)
}

// ToolFailedResponse builds a standard failed-tool response with an error
// message and a ToolCallUpdate marked ToolFailed.
func ToolFailedResponse(tc fantasy.ToolCall, err error) fantasy.ToolResponse {
	return ToolErrorResponse(err.Error(), acp.UpdateToolCallDelta(
		acp.ToolCallID(tc.ID),
		acp.WithStatus(acp.ToolFailed),
		acp.WithRawOutput(err.Error()),
	))
}
