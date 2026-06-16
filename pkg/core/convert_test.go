package core

import (
	"encoding/json"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

func TestUpdatesToMessages(t *testing.T) {
	tests := []struct {
		name    string
		updates []acp.SessionUpdate
		want    func(t *testing.T, msgs []fantasy.Message)
	}{
		{
			name:    "empty",
			updates: nil,
			want:    func(t *testing.T, msgs []fantasy.Message) { assertLen(t, msgs, 0) },
		},
		{
			name: "user message text",
			updates: []acp.SessionUpdate{
				acp.UpdateUserMessage(acp.TextBlock("hello"), acp.WithMessageID("m1")),
			},
			want: func(t *testing.T, msgs []fantasy.Message) {
				assertLen(t, msgs, 1)
				assertEqual(t, fantasy.MessageRoleUser, msgs[0].Role)
				assertTextPart(t, msgs[0].Content[0], "hello")
			},
		},
		{
			name: "agent message text",
			updates: []acp.SessionUpdate{
				acp.UpdateAgentMessage(acp.TextBlock("world"), acp.WithMessageID("m2")),
			},
			want: func(t *testing.T, msgs []fantasy.Message) {
				assertLen(t, msgs, 1)
				assertEqual(t, fantasy.MessageRoleAssistant, msgs[0].Role)
				assertTextPart(t, msgs[0].Content[0], "world")
			},
		},
		{
			name: "agent thought chunk",
			updates: []acp.SessionUpdate{
				acp.UpdateAgentThought(acp.TextBlock("thinking..."), acp.WithMessageID("m3")),
			},
			want: func(t *testing.T, msgs []fantasy.Message) {
				assertLen(t, msgs, 1)
				assertEqual(t, fantasy.MessageRoleAssistant, msgs[0].Role)
				assertReasoningPart(t, msgs[0].Content[0], "thinking...")
			},
		},
		{
			name: "tool call",
			updates: []acp.SessionUpdate{
				acp.UpdateToolCallStart(
					"tc1",
					acp.WithTitle("read"),
					acp.WithKind(acp.ToolRead),
					acp.WithRawInput(json.RawMessage(`{"path":"x.go"}`)),
				),
			},
			want: func(t *testing.T, msgs []fantasy.Message) {
				assertLen(t, msgs, 1)
				assertEqual(t, fantasy.MessageRoleAssistant, msgs[0].Role)
				assertToolCallPart(t, msgs[0].Content[0], "tc1", "read", `{"path":"x.go"}`)
			},
		},
		{
			name: "tool call result success",
			updates: []acp.SessionUpdate{
				acp.UpdateToolCallDelta("tc1", acp.WithRawOutput("result"), acp.WithTitle("read")),
			},
			want: func(t *testing.T, msgs []fantasy.Message) {
				assertLen(t, msgs, 1)
				assertEqual(t, fantasy.MessageRoleTool, msgs[0].Role)
				assertToolResultText(t, msgs[0].Content[0], "tc1", "result")
			},
		},
		{
			name: "tool call result failed",
			updates: []acp.SessionUpdate{
				acp.UpdateToolCallDelta(
					"tc1",
					acp.WithStatus(acp.ToolFailed),
					acp.WithRawOutput("fail"),
					acp.WithTitle("read"),
				),
			},
			want: func(t *testing.T, msgs []fantasy.Message) {
				assertLen(t, msgs, 1)
				assertEqual(t, fantasy.MessageRoleTool, msgs[0].Role)
				tr, ok := msgs[0].Content[0].(fantasy.ToolResultPart)
				if !ok {
					t.Fatalf("expected ToolResultPart, got %T", msgs[0].Content[0])
				}
				assertEqual(t, "tc1", tr.ToolCallID)
				_, ok = tr.Output.(fantasy.ToolResultOutputContentError)
				if !ok {
					t.Fatalf("expected error output, got %T", tr.Output)
				}
			},
		},
		{
			name: "tool call with empty title skipped",
			updates: []acp.SessionUpdate{
				acp.UpdateToolCallStart("tc1"),
			},
			want: func(t *testing.T, msgs []fantasy.Message) { assertLen(t, msgs, 0) },
		},
		{
			name: "agent thought with nil text skipped",
			updates: []acp.SessionUpdate{
				acp.UpdateAgentThought(acp.ContentBlock{}, acp.WithMessageID("m1")),
			},
			want: func(t *testing.T, msgs []fantasy.Message) { assertLen(t, msgs, 0) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := UpdatesToMessages(tt.updates)
			tt.want(t, msgs)
		})
	}
}

func TestPromptToMessage(t *testing.T) {
	t.Run("empty blocks", func(t *testing.T) {
		_, ok := PromptToMessage(nil)
		if ok {
			t.Fatal("expected false for nil blocks")
		}
	})
	t.Run("text", func(t *testing.T) {
		msg, ok := PromptToMessage([]acp.ContentBlock{acp.TextBlock("hello"), acp.TextBlock("world")})
		if !ok {
			t.Fatal("expected ok")
		}
		assertEqual(t, fantasy.MessageRoleUser, msg.Role)
		assertLen(t, msg.Content, 2)
		assertTextPart(t, msg.Content[0], "hello")
		assertTextPart(t, msg.Content[1], "world")
	})
	t.Run("image", func(t *testing.T) {
		msg, ok := PromptToMessage([]acp.ContentBlock{acp.ImageBlock("iVBORw==", "image/png")})
		if !ok {
			t.Fatal("expected ok")
		}
		assertEqual(t, fantasy.MessageRoleUser, msg.Role)
		fp, ok := msg.Content[0].(fantasy.FilePart)
		if !ok {
			t.Fatalf("expected FilePart, got %T", msg.Content[0])
		}
		assertEqual(t, "image/png", fp.MediaType)
		assertEqual(t, "iVBORw==", string(fp.Data))
	})
	t.Run("resource link", func(t *testing.T) {
		msg, ok := PromptToMessage([]acp.ContentBlock{acp.ResourceLinkBlock("link", "https://x.com")})
		if !ok {
			t.Fatal("expected ok")
		}
		assertTextPart(t, msg.Content[0], "https://x.com")
	})
}

func TestContentBlockToMessagePart(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		p := contentBlockToMessagePart(acp.TextBlock("hello"))
		assertTextPart(t, p, "hello")
	})
	t.Run("image", func(t *testing.T) {
		p := contentBlockToMessagePart(acp.ImageBlock("abc", "image/png"))
		fp, ok := p.(fantasy.FilePart)
		if !ok {
			t.Fatalf("expected FilePart, got %T", p)
		}
		assertEqual(t, "image/png", fp.MediaType)
		assertEqual(t, "abc", string(fp.Data))
	})
	t.Run("resource text", func(t *testing.T) {
		p := contentBlockToMessagePart(acp.ResourceBlock(acp.TextResource("urn:1", "data")))
		fp, ok := p.(fantasy.FilePart)
		if !ok {
			t.Fatalf("expected FilePart, got %T", p)
		}
		assertEqual(t, "", fp.MediaType)
		assertEqual(t, "data", string(fp.Data))
	})
	t.Run("resource blob", func(t *testing.T) {
		p := contentBlockToMessagePart(acp.ResourceBlock(acp.BlobResource("urn:2", "xyz")))
		fp, ok := p.(fantasy.FilePart)
		if !ok {
			t.Fatalf("expected FilePart, got %T", p)
		}
		assertEqual(t, "", fp.MediaType)
		assertEqual(t, "xyz", string(fp.Data))
	})
	t.Run("resource link", func(t *testing.T) {
		p := contentBlockToMessagePart(acp.ResourceLinkBlock("link", "https://x.com"))
		assertTextPart(t, p, "https://x.com")
	})
	t.Run("nil returns nil", func(t *testing.T) {
		p := contentBlockToMessagePart(acp.ContentBlock{})
		if p != nil {
			t.Fatalf("expected nil, got %T", p)
		}
	})
}

func TestToolNameToACP(t *testing.T) {
	tests := map[string]acp.ToolKind{
		"read":                acp.ToolRead,
		"write":               acp.ToolEdit,
		"edit":                acp.ToolEdit,
		"plan":                acp.ToolThink,
		"execute":             acp.ToolExecute,
		"terminal_create":     acp.ToolExecute,
		"terminal_output":     acp.ToolExecute,
		"terminal_wait":       acp.ToolExecute,
		"terminal_kill":       acp.ToolExecute,
		"terminal_release":    acp.ToolExecute,
		"unknown":               acp.ToolOther,
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			got := ToolNameToACP(name)
			if got != want {
				t.Errorf("ToolNameToACP(%q) = %v, want %v", name, got, want)
			}
		})
	}
}

func TestFinishReasonToACP(t *testing.T) {
	tests := map[fantasy.FinishReason]acp.StopReason{
		fantasy.FinishReasonStop:          acp.StopEndTurn,
		fantasy.FinishReasonLength:        acp.StopMaxTokens,
		fantasy.FinishReasonToolCalls:     acp.StopEndTurn,
		fantasy.FinishReasonContentFilter: acp.StopRefusal,
		fantasy.FinishReasonError:         acp.StopEndTurn,
		fantasy.FinishReason("unknown"):   acp.StopEndTurn,
	}
	for reason, want := range tests {
		t.Run(string(reason), func(t *testing.T) {
			got := FinishReasonToACP(reason)
			if got != want {
				t.Errorf("FinishReasonToACP(%v) = %v, want %v", reason, got, want)
			}
		})
	}
}

func TestTypeToACP(t *testing.T) {
	tests := map[catwalk.Type]acp.LlmProtocol{
		catwalk.TypeAnthropic:    acp.LlmProtocolAnthropic,
		catwalk.TypeGoogle:       acp.LlmProtocolGoogle,
		catwalk.TypeAzure:        acp.LlmProtocolAzure,
		catwalk.TypeBedrock:      acp.LlmProtocolBedrock,
		catwalk.TypeVertexAI:     acp.LlmProtocolVertex,
		catwalk.TypeOpenRouter:   acp.LlmProtocolOpenRouter,
		catwalk.TypeOpenAI:       acp.LlmProtocolOpenAI,
		catwalk.Type("unknown"):  acp.LlmProtocolOpenAICompat,
	}
	for typ, want := range tests {
		t.Run(string(typ), func(t *testing.T) {
			got := TypeToACP(typ)
			if got != want {
				t.Errorf("TypeToACP(%q) = %v, want %v", typ, got, want)
			}
		})
	}
}

func TestUsageToACP(t *testing.T) {
	got := UsageToACP(fantasy.Usage{})
	if got != nil {
		t.Fatal("expected nil for zero usage")
	}

	got = UsageToACP(fantasy.Usage{
		InputTokens:       100,
		OutputTokens:      50,
		TotalTokens:       150,
		ReasoningTokens:   20,
		CacheReadTokens:   10,
		CacheCreationTokens: 5,
	})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	assertEqual(t, uint64(100), got.InputTokens)
	assertEqual(t, uint64(50), got.OutputTokens)
	assertEqual(t, uint64(150), got.TotalTokens)
	if got.ThoughtTokens == nil {
		t.Fatal("expected ThoughtTokens")
	}
	assertEqual(t, uint64(20), *got.ThoughtTokens)
	if got.CachedReadTokens == nil {
		t.Fatal("expected CachedReadTokens")
	}
	assertEqual(t, uint64(10), *got.CachedReadTokens)
	if got.CachedWriteTokens == nil {
		t.Fatal("expected CachedWriteTokens")
	}
	assertEqual(t, uint64(5), *got.CachedWriteTokens)
}

// helpers

func assertLen[T any](t *testing.T, s []T, want int) {
	t.Helper()
	if len(s) != want {
		t.Fatalf("len = %d, want %d", len(s), want)
	}
}

func assertEqual[T comparable](t *testing.T, want, got T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func assertTextPart(t *testing.T, p fantasy.MessagePart, want string) {
	t.Helper()
	tp, ok := p.(fantasy.TextPart)
	if !ok {
		t.Fatalf("expected TextPart, got %T", p)
	}
	if tp.Text != want {
		t.Fatalf("Text = %q, want %q", tp.Text, want)
	}
}

func assertReasoningPart(t *testing.T, p fantasy.MessagePart, want string) {
	t.Helper()
	rp, ok := p.(fantasy.ReasoningPart)
	if !ok {
		t.Fatalf("expected ReasoningPart, got %T", p)
	}
	if rp.Text != want {
		t.Fatalf("Text = %q, want %q", rp.Text, want)
	}
}

func assertToolCallPart(t *testing.T, p fantasy.MessagePart, wantID, wantName, wantInput string) {
	t.Helper()
	tc, ok := p.(fantasy.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", p)
	}
	if tc.ToolCallID != wantID {
		t.Fatalf("ToolCallID = %q, want %q", tc.ToolCallID, wantID)
	}
	if tc.ToolName != wantName {
		t.Fatalf("ToolName = %q, want %q", tc.ToolName, wantName)
	}
	if tc.Input != wantInput {
		t.Fatalf("Input = %q, want %q", tc.Input, wantInput)
	}
}

func assertToolResultText(t *testing.T, p fantasy.MessagePart, wantID, wantOutput string) {
	t.Helper()
	tr, ok := p.(fantasy.ToolResultPart)
	if !ok {
		t.Fatalf("expected ToolResultPart, got %T", p)
	}
	if tr.ToolCallID != wantID {
		t.Fatalf("ToolCallID = %q, want %q", tr.ToolCallID, wantID)
	}
	tc, ok := tr.Output.(fantasy.ToolResultOutputContentText)
	if !ok {
		t.Fatalf("expected text output, got %T", tr.Output)
	}
	if tc.Text != wantOutput {
		t.Fatalf("Text = %q, want %q", tc.Text, wantOutput)
	}
}
