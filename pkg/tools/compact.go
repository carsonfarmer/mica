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

type CompactToolInput struct {
	Instructions string `json:"instructions"`
}

const compactPrompt = `Summarize the conversation above into a concise context checkpoint. Preserve:

- The user's goal and any constraints they set
- Key decisions made and their rationale
- Exact file paths, function names, and error messages
- What's done, what's in progress, what's blocked
- Next steps in priority order
- Any critical context needed to continue work

Format freely — be brief but complete. Do not continue the conversation. Only output the summary.`

// Compact runs the compaction algorithm directly (used by both the compact
// tool and the /compact slash command). On success the event chain is
// replaced with summary + tail.
func Compact(ctx context.Context, store storage.Store[*core.AgentSession], reg *core.Registry, instructions string) (string, error) {
	sess := core.SessionFrom(ctx)
	before := sess.Usage.TotalTokens
	events, err := store.Load(ctx, sess.SessionID)
	if err != nil {
		return "", fmt.Errorf("load events: %w", err)
	}
	keep := max(len(events)*2/5, 5)
	if keep >= len(events) {
		return "", fmt.Errorf("session too short to compact")
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
		return "", fmt.Errorf("resolve model: %w", err)
	}
	prompt := compactPrompt
	if instructions != "" {
		prompt = instructions + "\n\n" + compactPrompt
	}
	resp, err := model.Generate(ctx, fantasy.Call{
		Prompt: []fantasy.Message{
			{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: transcript.String() + "\n\n" + prompt},
			}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("summarize: %w", err)
	}
	var summary strings.Builder
	for _, c := range resp.Content {
		if t, ok := c.(fantasy.TextContent); ok {
			summary.WriteString(t.Text)
		}
	}
	if summary.Len() == 0 {
		return "", fmt.Errorf("empty summary from model")
	}

	final := "## Compaction Summary:\n" + summary.String()
	if s := toolSummary(events, sess.CWD); s != "" {
		final += "\n\n### Changes\n" + s
	}

	summaryUpd := acp.UpdateUserMessage(
		acp.TextBlock(final),
		acp.WithMessageID(acp.NewUUID()),
	)
	root, err := store.Append(ctx, sess.SessionID, summaryUpd, nil)
	if err != nil {
		return "", fmt.Errorf("append summary: %w", err)
	}
	for _, upd := range kept {
		root, err = store.Append(ctx, sess.SessionID, upd, root)
		if err != nil {
			return "", fmt.Errorf("re-append event: %w", err)
		}
	}
	if err := store.Commit(ctx, sess.SessionID, *root); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return fmt.Sprintf("Compacted from %d tokens into %d-char summary. %d events kept.", before, summary.Len(), keep), nil
}

// CompactTool wraps Compact as a Fantasy agent tool.
func CompactTool(store storage.Store[*core.AgentSession], reg *core.Registry) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"compact",
		"Compact the conversation history to save context space. Summarizes older messages and replaces them with a summary in the event chain.",
		func(ctx context.Context, in CompactToolInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			result, err := Compact(ctx, store, reg, in.Instructions)
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID), acp.WithStatus(acp.ToolCompleted), acp.WithTitle("compact"), acp.WithRawOutput(result))
			return ToolResponse(result, upd), nil
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
