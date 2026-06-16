package tools

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/mica/pkg/core"
)

type CompactToolInput struct{}

const compactPrompt = `Summarize the conversation above into a concise context checkpoint. Preserve:

- The user's goal and any constraints they set
- Key decisions made and their rationale
- Exact file paths, function names, and error messages
- What's done, what's in progress, what's blocked
- Next steps in priority order
- Any critical context needed to continue work

Format freely — be brief but complete. Do not continue the conversation. Only output the summary.`

// CompactTool replaces older conversation history with an LLM-generated
// summary, preserving the most recent ~40% of events as-is.
//
// Algorithm:
//  1. Load all session events from the store.
//  2. Keep the last max(40%, 5) events as the tail; everything before is
//     summarized.
//  3. Serialize all events (full history) as a transcript in [role]: content
//     format so the model sees complete context, not structured messages.
//  4. Send transcript + compactPrompt to the current session model.
//  5. Append a toolSummary (tool:path counts with locations) to the model's
//     response.
//  6. Write the combined summary as a new root event (parent=nil).
//  7. Re-append the kept tail events under this new root.
//  8. Commit, making summary+tail the new event chain.
func CompactTool(store storage.Store[*core.AgentSession], reg *core.Registry) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"compact",
		"Compact the conversation history to save context space. Summarizes older messages and replaces them with a summary in the event chain.",
		func(ctx context.Context, _ CompactToolInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sess := core.SessionFrom(ctx)
			events, err := store.Load(ctx, sess.SessionInfo.SessionID)
			if err != nil {
				return ToolFailedResponse(tc, fmt.Errorf("load events: %w", err)), nil
			}
			keep := max(len(events)*2/5, 5)
			if keep >= len(events) {
				return ToolResponse("Nothing to compact — session is already short.", acp.UpdateToolCallDelta(
					acp.ToolCallID(tc.ID), acp.WithStatus(acp.ToolCompleted), acp.WithTitle("compact (nothing to do)"),
				)), nil
			}
			kept := events[len(events)-keep:]

			var transcript strings.Builder
			for _, m := range core.UpdatesToMessages(events) {
				for _, p := range m.Content {
					fmt.Fprintf(&transcript, "[%s]: %s\n", m.Role, p)
				}
			}

			model, err := reg.Resolve(ctx, sess.Model)
			if err != nil {
				return ToolFailedResponse(tc, fmt.Errorf("resolve model: %w", err)), nil
			}
			resp, err := model.Generate(ctx, fantasy.Call{
				Prompt: []fantasy.Message{
					{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{
						fantasy.TextPart{Text: transcript.String() + "\n\n" + compactPrompt},
					}},
				},
			})
			if err != nil {
				return ToolFailedResponse(tc, fmt.Errorf("summarize: %w", err)), nil
			}
			var summary strings.Builder
			for _, c := range resp.Content {
				if t, ok := c.(fantasy.TextContent); ok {
					summary.WriteString(t.Text)
				}
			}
			if summary.Len() == 0 {
				return ToolFailedResponse(tc, fmt.Errorf("empty summary from model")), nil
			}

			final := "## Compaction Summary:\n" + summary.String()
			if s := toolSummary(events, sess.SessionInfo.CWD); s != "" {
				final += "\n\n### Changes\n" + s
			}

			summaryUpd := acp.UpdateUserMessage(
				acp.TextBlock(final),
				acp.WithMessageID(acp.NewUUID()),
			)
			root, err := store.Append(ctx, sess.SessionInfo.SessionID, summaryUpd, nil)
			if err != nil {
				return ToolFailedResponse(tc, fmt.Errorf("append summary: %w", err)), nil
			}
			for _, upd := range kept {
				root, err = store.Append(ctx, sess.SessionInfo.SessionID, upd, root)
				if err != nil {
					return ToolFailedResponse(tc, fmt.Errorf("re-append event: %w", err)), nil
				}
			}
			if err := store.Commit(ctx, sess.SessionInfo.SessionID, *root); err != nil {
				return ToolFailedResponse(tc, fmt.Errorf("commit: %w", err)), nil
			}
			msg := fmt.Sprintf("Compacted %d events into summary (%d chars). %d events kept.",
				len(events)-keep, summary.Len(), keep)
			upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID), acp.WithStatus(acp.ToolCompleted), acp.WithTitle("compact"), acp.WithRawOutput(msg))
			return ToolResponse(msg, upd), nil
		},
	)
}

func toolSummary(events []acp.SessionUpdate, cwd string) string {
	counts := map[string]int{}
	for _, e := range events {
		tc := e.ToolCallUpdate
		if tc == nil || tc.Title == "" {
			continue
		}
		name, _, _ := strings.Cut(tc.Title, " ")
		if locs := tc.Locations; locs != nil {
			for _, loc := range locs {
				counts[name+":"+RelPath(cwd, loc.Path)]++
			}
		} else {
			counts[tc.Title]++
		}
	}
	if len(counts) == 0 {
		return ""
	}
	var b strings.Builder
	for k, v := range counts {
		fmt.Fprintf(&b, "- %s x %d\n", k, v)
	}
	return b.String()
}
