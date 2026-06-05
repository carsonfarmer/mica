package llm

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/carsonfarmer/go-acp-sdk"
)

type ctxKey string

const (
	ctxClient    ctxKey = "acp-client"
	ctxSessionID ctxKey = "acp-session-id"
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

// WrapCodeBlock wraps content in a markdown code block using the file extension as language.
func WrapCodeBlock(path, content string) string {
	ext := filepath.Ext(path)
	lang := ext
	if len(lang) > 0 {
		lang = lang[1:]
	}
	return fmt.Sprintf("```%s\n%s\n```", lang, content)
}
