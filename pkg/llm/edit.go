package llm

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

const ToolNameEdit = "edit"

// Edit specifies a line-based replacement within a file.
type Edit struct {
	Start int    `json:"start" description:"0-based start line index"`
	End   int    `json:"end" description:"0-based end line index (exclusive)"`
	New   string `json:"new" description:"Replacement text (can be multi-line)"`
}

// EditInput is the input for the edit tool.
type EditInput struct {
	Path  string `json:"path" description:"Path to the file to edit (relative or absolute)"`
	Edits []Edit `json:"edits" description:"One or more line-based replacements"`
}

// EditTool creates a tool that edits a file via line-based replacements.
func EditTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		ToolNameEdit,
		"Edit a file with line-based replacements. Each edit specifies a line range (start, end) and replacement text.",
		func(ctx context.Context, in EditInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			sid := SessionFrom(ctx)

			var oldContent string
			if resp, err := client.ReadTextFile(ctx, &acp.ReadTextFileRequest{
				SessionID: sid, Path: in.Path,
			}); err == nil {
				oldContent = resp.Content
			}

			newContent, err := applyEdits(oldContent, in.Edits)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			_, err = client.WriteTextFile(ctx, &acp.WriteTextFileRequest{
				SessionID: sid,
				Path:      in.Path,
				Content:   newContent,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID))
			upd.ToolCallUpdate.RawInput = in
			upd.ToolCallUpdate.Locations = []acp.ToolCallLocation{{Path: in.Path}}
			upd.ToolCallUpdate.Content = []acp.ToolCallContent{acp.ToolDiff(in.Path, newContent, oldContent)}
			client.SessionUpdate(ctx, &acp.SessionNotification{SessionID: sid, Update: upd})

			return fantasy.ToolResponse{
				Type:     "text",
				Content:  fmt.Sprintf("Successfully applied %d edit(s) to %s.", len(in.Edits), in.Path),
				Metadata: oldContent,
			}, nil
		},
	)
}

// applyEdits applies line-based edits to content. Edits are sorted by line
// and applied left-to-right in a single pass.
func applyEdits(content string, edits []Edit) (string, error) {
	if len(edits) == 0 {
		return "", fmt.Errorf("no edits provided")
	}

	lines := strings.Split(content, "\n")

	sorted := make([]Edit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })

	var result []string
	prev := 0
	for i, e := range sorted {
		if e.Start < 0 || e.End > len(lines) || e.Start > e.End {
			return "", fmt.Errorf("edit %d: invalid line range [%d, %d) for file with %d lines", i, e.Start, e.End, len(lines))
		}
		if e.Start < prev {
			return "", fmt.Errorf("overlapping edits at lines [%d,%d)", e.Start, e.End)
		}
		result = append(result, lines[prev:e.Start]...)
		newLines := strings.Split(e.New, "\n")
		if e.New == "" {
			newLines = nil
		}
		result = append(result, newLines...)
		prev = e.End
	}
	result = append(result, lines[prev:]...)
	return strings.Join(result, "\n"), nil
}
