package llm

import (
	"context"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

const ToolNameRead = "read"

// ReadFileInput is the input for the read_file tool.
type ReadFileInput struct {
	Path  string `json:"path" description:"Absolute path to the file to read"`
	Line  int    `json:"line,omitempty" description:"Line number to start reading from (1-indexed)"`
	Limit int    `json:"limit,omitempty" description:"Maximum number of lines to read"`
}

// ReadFileTool creates a tool that reads a file via the ACP client.
func ReadFileTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		ToolNameRead,
		"Read a file from the local filesystem.",
		func(ctx context.Context, in ReadFileInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			resp, err := ClientFrom(ctx).ReadTextFile(ctx, &acp.ReadTextFileRequest{
				SessionID: SessionFrom(ctx),
				Path:      in.Path,
				Line:      in.Line,
				Limit:     in.Limit,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}

			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithTitle(ToolNameRead+" "+RelPath(ctx, in.Path)),
				acp.WithRawOutput(resp.Content),
				acp.WithRawInput(in),
				acp.WithLocations(acp.ToolCallLocation{Path: in.Path, Line: in.Line}),
				acp.WithToolContent(acp.ToolContent(acp.TextBlock(WrapCodeBlock(in.Path, resp.Content)))),
			)
			return ToolResponse(resp.Content, upd), nil
		},
	)
}
