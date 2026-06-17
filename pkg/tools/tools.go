package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/mica/pkg/core"
)

// CheckPermission checks whether a tool call is allowed under the current
// session mode. In normal mode it always returns nil. In safe mode it
// checks stored permission rules and prompts the user if needed.
func CheckPermission(ctx context.Context, tc fantasy.ToolCall, target string) error {
	s := core.SessionFrom(ctx)
	if s == nil || s.Mode != core.ModeSafe {
		return nil
	}
	switch (&s.Permissions).Check(target) {
	case agentutil.PermAllow:
		return nil
	case agentutil.PermDeny:
		return fmt.Errorf("permission denied: %s", target)
	}

	client := core.ClientFrom(ctx)
	kind := core.ToolNameToACP(tc.Name)
	resp, err := client.RequestPermission(ctx, &acp.RequestPermissionRequest{
		SessionID: s.SessionID,
		ToolCall: acp.ToolCallUpdate{
			ToolCallID: acp.ToolCallID(tc.ID),
			Title:      tc.Name + " " + strings.TrimPrefix(target, tc.Name+":"),
			Kind:       &kind,
		},
		Options: []acp.PermissionOption{
			{OptionID: string(acp.PermAllowOnce), Name: "Allow once", Kind: acp.PermAllowOnce},
			{OptionID: string(acp.PermAllowAlways), Name: "Allow always", Kind: acp.PermAllowAlways},
			{OptionID: string(acp.PermRejectOnce), Name: "Reject once", Kind: acp.PermRejectOnce},
			{OptionID: string(acp.PermRejectAlways), Name: "Reject always", Kind: acp.PermRejectAlways},
		},
	})
	if err != nil {
		return err
	}
	if resp.Outcome.Selected != nil {
		switch acp.PermissionOptionKind(resp.Outcome.Selected.OptionID) {
		case acp.PermAllowAlways:
			(&s.Permissions).AddRule(target, agentutil.PermAllow)
		case acp.PermRejectAlways:
			(&s.Permissions).AddRule(target, agentutil.PermDeny)
		case acp.PermRejectOnce:
			return fmt.Errorf("permission denied: %s", target)
		}
	}
	return nil
}

// RelPath returns p relative to cwd, or p unchanged.
func RelPath(cwd, p string) string {
	if cwd == "" {
		return p
	}
	if r, err := filepath.Rel(cwd, p); err == nil {
		return r
	}
	return p
}

// WrapCodeBlock wraps content in a markdown code block using the file extension as language.
func WrapCodeBlock(path, content string) string {
	ext := filepath.Ext(path)
	lang := ext
	if len(lang) > 0 {
		lang = lang[1:]
	}
	return fmt.Sprintf("````%s\n%s\n````", lang, content)
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
