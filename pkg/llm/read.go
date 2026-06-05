package llm

import (
	"context"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

const ToolNameReadFile = "read_file"

// ReadFileInput is the input for the read_file tool.
type ReadFileInput struct {
	Path  string `json:"path" description:"Absolute path to the file to read"`
	Line  int    `json:"line,omitempty" description:"Line number to start reading from (1-indexed)"`
	Limit int    `json:"limit,omitempty" description:"Maximum number of lines to read"`
}

// ReadFileTool creates a tool that reads a file via the ACP client.
func ReadFileTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		ToolNameReadFile,
		"Read a file from the local filesystem.",
		func(ctx context.Context, in ReadFileInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			resp, err := ClientFrom(ctx).ReadTextFile(ctx, &acp.ReadTextFileRequest{
				SessionID: SessionFrom(ctx),
				Path:      in.Path,
				Line:      in.Line,
				Limit:     in.Limit,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID))
			upd.ToolCallUpdate.RawInput = in
			upd.ToolCallUpdate.Locations = []acp.ToolCallLocation{{Path: in.Path, Line: in.Line}}
			upd.ToolCallUpdate.Content = []acp.ToolCallContent{acp.ToolContent(acp.TextBlock(WrapCodeBlock(in.Path, resp.Content)))}
			ClientFrom(ctx).SessionUpdate(ctx, &acp.SessionNotification{SessionID: SessionFrom(ctx), Update: upd})

			return fantasy.NewTextResponse(resp.Content), nil
		},
	)
}
