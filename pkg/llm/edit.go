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

// Edit specifies a 1-based line replacement within a file. Start and End are
// both inclusive. To replace line 5 alone: Start=5, End=5. To delete lines
// 5-7: Start=5, End=7, New="".
type Edit struct {
	Start int    `json:"start" description:"1-based start line (inclusive)"`
	End   int    `json:"end" description:"1-based end line (inclusive)"`
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
				return ToolFailedResponse(tc, err), nil
			}

			_, err = client.WriteTextFile(ctx, &acp.WriteTextFileRequest{
				SessionID: sid, Path: in.Path, Content: newContent,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}

			output := fmt.Sprintf("Successfully applied %d edit(s) to %s.", len(in.Edits), in.Path)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithTitle(ToolNameEdit+" "+RelPath(ctx, in.Path)),
				acp.WithRawOutput(output),
				acp.WithRawInput(in),
				acp.WithLocations(acp.ToolCallLocation{Path: in.Path}),
				acp.WithToolContent(acp.ToolDiff(in.Path, newContent, oldContent)),
			)
			return ToolResponse(output, upd), nil
		},
	)
}

// applyEdits applies 1-based edits to content. Edits are sorted by start line
// and applied left-to-right in a single pass.
func applyEdits(content string, edits []Edit) (string, error) {
	if len(edits) == 0 {
		return "", fmt.Errorf("no edits provided")
	}

	lines := strings.Split(content, "\n")
	sort.Slice(edits, func(i, j int) bool { return edits[i].Start < edits[j].Start })

	var result []string
	prev := 0
	for i, e := range edits {
		if e.Start < 1 || e.End > len(lines)+1 || e.Start > e.End {
			return "", fmt.Errorf("edit %d: invalid line range [%d,%d] for file with %d lines", i, e.Start, e.End, len(lines))
		}
		start0 := e.Start - 1
		end0 := e.End
		if start0 < prev {
			return "", fmt.Errorf("edit %d: overlapping edits at line %d", i, e.Start)
		}
		result = append(result, lines[prev:start0]...)
		if e.New != "" {
			result = append(result, strings.Split(e.New, "\n")...)
		}
		prev = end0
	}
	if prev < len(lines) {
		result = append(result, lines[prev:]...)
	}
	return strings.Join(result, "\n"), nil
}
