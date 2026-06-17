package tools

import (
	"context"
	"path/filepath"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/mica/pkg/core"
)

// ReadInput is the input for the read tool.
type ReadInput struct {
	Path  string `json:"path" description:"Absolute path to the file to read"`
	Line  int    `json:"line,omitempty" description:"Line number to start reading from (1-indexed)"`
	Limit int    `json:"limit,omitempty" description:"Maximum number of lines to read"`
}

// ReadTool reads a file. In safe mode, requests read permission on the
// parent directory (folder-level gating). Returns the file content as text
// wrapped in a markdown code block.
func ReadTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		"read",
		"Read a file from the local filesystem.",
		func(ctx context.Context, in ReadInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := CheckPermission(ctx, tc, "read:"+filepath.Dir(in.Path)+"/"); err != nil {
				return ToolErrorResponse(err.Error()), nil
			}
			sess := core.SessionFrom(ctx)
			resp, err := core.ClientFrom(ctx).ReadTextFile(ctx, &acp.ReadTextFileRequest{
				SessionID: sess.SessionID,
				Path: in.Path, Line: in.Line, Limit: in.Limit,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithTitle("read"+" "+RelPath(sess.CWD, in.Path)),
				acp.WithRawOutput(resp.Content),
				acp.WithRawInput(in),
				acp.WithLocations(acp.ToolCallLocation{Path: in.Path, Line: in.Line}),
				acp.WithToolContent(acp.ToolContent(acp.TextBlock(WrapCodeBlock(in.Path, resp.Content)))),
			)
			return ToolResponse(resp.Content, upd), nil
		},
	)
}
