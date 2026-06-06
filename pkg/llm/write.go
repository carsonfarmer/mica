package llm

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

const ToolNameWrite = "write"

// WriteFileInput is the input for the write_file tool.
type WriteFileInput struct {
	Path    string `json:"path" description:"Absolute path to the file to write"`
	Content string `json:"content" description:"Content to write to the file"`
}

// WriteFileTool creates a tool that writes a file via the ACP client.
func WriteFileTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		ToolNameWrite,
		"Write content to a file on the local filesystem.",
		func(ctx context.Context, in WriteFileInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			_, err := ClientFrom(ctx).WriteTextFile(ctx, &acp.WriteTextFileRequest{
				SessionID: SessionFrom(ctx),
				Path:      in.Path,
				Content:   in.Content,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}

			output := fmt.Sprintf("Successfully wrote %d bytes to %s.", len(in.Content), in.Path)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithTitle(ToolNameWrite+" "+RelPath(ctx, in.Path)),
				acp.WithRawOutput(output),
				acp.WithRawInput(in),
				acp.WithLocations(acp.ToolCallLocation{Path: in.Path}),
				acp.WithToolContent(acp.ToolContent(acp.TextBlock(WrapCodeBlock(in.Path, in.Content)))),
			)
			return ToolResponse(output, upd), nil
		},
	)
}
