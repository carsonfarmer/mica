package llm

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

const ToolNameWriteFile = "write_file"

// WriteFileInput is the input for the write_file tool.
type WriteFileInput struct {
	Path    string `json:"path" description:"Absolute path to the file to write"`
	Content string `json:"content" description:"Content to write to the file"`
}

// WriteFileTool creates a tool that writes a file via the ACP client.
func WriteFileTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		ToolNameWriteFile,
		"Write content to a file on the local filesystem.",
		func(ctx context.Context, in WriteFileInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			sid := SessionFrom(ctx)

			_, err := client.WriteTextFile(ctx, &acp.WriteTextFileRequest{
				SessionID: sid,
				Path:      in.Path,
				Content:   in.Content,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID))
			upd.ToolCallUpdate.RawInput = in
			upd.ToolCallUpdate.Locations = []acp.ToolCallLocation{{Path: in.Path}}
			upd.ToolCallUpdate.Content = []acp.ToolCallContent{acp.ToolContent(acp.TextBlock(WrapCodeBlock(in.Path, in.Content)))}
			client.SessionUpdate(ctx, &acp.SessionNotification{SessionID: sid, Update: upd})

			return fantasy.NewTextResponse(fmt.Sprintf("Successfully wrote %d bytes to %s.", len(in.Content), in.Path)), nil
		},
	)
}
