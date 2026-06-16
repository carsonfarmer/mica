package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/mica/pkg/core"
)

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

// EditTool applies 1-based line-range replacements to a file. Reads the
// file first (silently treating missing as empty), applies edits sorted by
// start line in a single left-to-right pass, then writes the result.
// Produces a unified diff in the tool content.
func EditTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		"edit",
		"Edit a file with line-based replacements. Each edit specifies a line range (start, end) and replacement text.",
		func(ctx context.Context, in EditInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := CheckPermission(ctx, tc, "edit:"+in.Path); err != nil {
				return ToolErrorResponse(err.Error()), nil
			}
			info := core.SessionFrom(ctx).SessionInfo
			var oldContent string
			if resp, err := core.ClientFrom(ctx).ReadTextFile(ctx, &acp.ReadTextFileRequest{
				SessionID: info.SessionID, Path: in.Path,
			}); err == nil {
				oldContent = resp.Content
			}
			newContent, err := applyEdits(oldContent, in.Edits)
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			if _, err = core.ClientFrom(ctx).WriteTextFile(ctx, &acp.WriteTextFileRequest{
				SessionID: info.SessionID, Path: in.Path, Content: newContent,
			}); err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			output := fmt.Sprintf("Successfully applied %d edit(s) to %s.", len(in.Edits), in.Path)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithTitle("edit"+" "+RelPath(info.CWD, in.Path)),
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
		if e.Start-1 < prev {
			return "", fmt.Errorf("edit %d: overlapping edits at line %d", i, e.Start)
		}
		result = append(result, lines[prev:e.Start-1]...)
		if e.New != "" {
			result = append(result, strings.Split(e.New, "\n")...)
		}
		prev = e.End
	}
	if prev < len(lines) {
		result = append(result, lines[prev:]...)
	}
	return strings.Join(result, "\n"), nil
}
