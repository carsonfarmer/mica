package tools

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/mica/pkg/core"
)

// WriteInput is the input for the write tool.
type WriteInput struct {
	Path    string `json:"path" description:"Absolute path to the file to write"`
	Content string `json:"content" description:"Content to write to the file"`
}

// WriteTool overwrites a file. In safe mode, requests write permission on
// the exact path. Returns confirmation with byte count and a markdown code
// block of the written content.
func WriteTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		"write",
		"Write content to a file on the local filesystem.",
		func(ctx context.Context, in WriteInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := CheckPermission(ctx, tc, "write:"+in.Path); err != nil {
				return ToolErrorResponse(err.Error()), nil
			}
			info := core.SessionFrom(ctx).SessionInfo
			if _, err := core.ClientFrom(ctx).WriteTextFile(ctx, &acp.WriteTextFileRequest{
				SessionID: info.SessionID,
				Path: in.Path, Content: in.Content,
			}); err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			output := fmt.Sprintf("Successfully wrote %d bytes to %s.", len(in.Content), in.Path)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithTitle("write"+" "+RelPath(info.CWD, in.Path)),
				acp.WithRawOutput(output),
				acp.WithRawInput(in),
				acp.WithLocations(acp.ToolCallLocation{Path: in.Path}),
				acp.WithToolContent(acp.ToolContent(acp.TextBlock(WrapCodeBlock(in.Path, in.Content)))),
			)
			return ToolResponse(output, upd), nil
		},
	)
}
