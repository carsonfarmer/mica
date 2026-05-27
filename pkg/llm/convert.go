package llm

import (
	"encoding/json"
	"fmt"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

// ACP → Fantasy (building LLM prompts from stored history)

// HistoryToMessages converts ACP session updates into Fantasy messages
// suitable for use as conversation history in an LLM prompt.
// Only message-carrying variants are converted (user/agent/thought chunks,
// tool calls and results). Other update types (plan, mode, config, usage, etc.)
// are transient notifications and not part of the conversation history.
func UpdatesToMessages(updates []acp.SessionUpdate) []fantasy.Message {
	var msgs []fantasy.Message
	for _, u := range updates {
		switch {
		case u.UserMessageChunk != nil:
			if m := convertUserChunk(u.UserMessageChunk); m != nil {
				msgs = append(msgs, *m)
			}
		case u.AgentMessageChunk != nil:
			if m := convertAgentChunk(u.AgentMessageChunk); m != nil {
				msgs = append(msgs, *m)
			}
		case u.AgentThoughtChunk != nil:
			if m := convertThoughtChunk(u.AgentThoughtChunk); m != nil {
				msgs = append(msgs, *m)
			}
		case u.ToolCall != nil:
			if m := convertToolCall(u.ToolCall); m != nil {
				msgs = append(msgs, *m)
			}
		case u.ToolCallUpdate != nil:
			if m := convertToolCallUpdate(u.ToolCallUpdate); m != nil {
				msgs = append(msgs, *m)
			}
		}
	}
	return msgs
}

func convertUserChunk(c *acp.SessionUpdateUserMessageChunk) *fantasy.Message {
	if p := blockToPart(c.Content); p != nil {
		return &fantasy.Message{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{p}}
	}
	return nil
}

func convertAgentChunk(c *acp.SessionUpdateAgentMessageChunk) *fantasy.Message {
	if p := blockToPart(c.Content); p != nil {
		return &fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{p}}
	}
	return nil
}

func convertThoughtChunk(c *acp.SessionUpdateAgentThoughtChunk) *fantasy.Message {
	return &fantasy.Message{
		Role:    fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{fantasy.ReasoningPart{Text: c.Content.Text.Text}},
	}
}

func convertToolCall(tc *acp.SessionUpdateToolCall) *fantasy.Message {
	input, _ := json.Marshal(tc.RawInput)
	return &fantasy.Message{
		Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{
			fantasy.ToolCallPart{ToolCallID: tc.ToolCallID, ToolName: tc.Title, Input: string(input)},
		},
	}
}

func convertToolCallUpdate(tu *acp.SessionUpdateToolCallUpdate) *fantasy.Message {
	var result fantasy.ToolResultOutputContent
	switch {
	case tu.Status != nil && *tu.Status == acp.ToolFailed:
		result = fantasy.ToolResultOutputContentError{Error: fmt.Errorf("tool call failed")}
	case len(tu.Content) > 0:
		result = toolContentToResult(tu.Content)
	case tu.RawOutput != nil:
		if s, ok := tu.RawOutput.(string); ok {
			result = fantasy.ToolResultOutputContentText{Text: s}
		}
	}
	return &fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{ToolCallID: tu.ToolCallID, Output: result},
		},
	}
}

func toolContentToResult(blocks []acp.ToolCallContent) fantasy.ToolResultOutputContent {
	for _, b := range blocks {
		if b.Content != nil {
			switch {
			case b.Content.Content.Text != nil:
				return fantasy.ToolResultOutputContentText{Text: b.Content.Content.Text.Text}
			case b.Content.Content.Image != nil:
				return fantasy.ToolResultOutputContentMedia{
					Data:      b.Content.Content.Image.Data,
					MediaType: b.Content.Content.Image.MimeType,
				}
			}
		}
		if b.Diff != nil || b.Terminal != nil {
			if raw, err := json.Marshal(b); err == nil {
				return fantasy.ToolResultOutputContentText{Text: string(raw)}
			}
		}
	}
	return nil
}

// PromptToMessage converts ACP content blocks into a Fantasy user message.
func PromptToMessage(blocks []acp.ContentBlock) (fantasy.Message, bool) {
	parts := make([]fantasy.MessagePart, 0, len(blocks))
	for _, b := range blocks {
		if p := blockToPart(b); p != nil {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return fantasy.Message{}, false
	}
	return fantasy.Message{
		Role:    fantasy.MessageRoleUser,
		Content: parts,
	}, true
}

// session updates for persistence (avoids round-tripping through Fantasy).

func blockToPart(b acp.ContentBlock) fantasy.MessagePart {
	switch {
	case b.Text != nil:
		return fantasy.TextPart{Text: b.Text.Text}
	case b.Image != nil:
		return fantasy.FilePart{
			Data:      []byte(b.Image.Data),
			MediaType: b.Image.MimeType,
		}
	}
	return nil
}

// finishReasonToACP maps Fantasy finish reasons to ACP stop reasons.
func FinishReasonToACP(fr fantasy.FinishReason) acp.StopReason {
	switch fr {
	case fantasy.FinishReasonStop:
		return acp.StopEndTurn
	case fantasy.FinishReasonLength:
		return acp.StopMaxTokens
	case fantasy.FinishReasonToolCalls:
		return acp.StopEndTurn // ACP doesn't have a tool-calls stop reason; end_turn is fine
	case fantasy.FinishReasonContentFilter:
		return acp.StopRefusal
	case fantasy.FinishReasonError:
		return acp.StopEndTurn // closest mapping
	default:
		return acp.StopEndTurn
	}
}

// toolKind maps a tool name to its ACP ToolKind.
func ToolNameToACP(name string) acp.ToolKind {
	switch name {
	case "read_file":
		return acp.ToolRead
	case "write_file":
		return acp.ToolEdit
	case "execute_command", "terminal_create", "terminal_output", "terminal_wait", "terminal_kill", "terminal_release":
		return acp.ToolExecute
	case "plan":
		return acp.ToolThink
	default:
		return acp.ToolOther
	}
}

// MessageToACP converts a Fantasy message to ACP session updates.
func MessageToACP(msg fantasy.Message) []acp.SessionUpdate {
	var updates []acp.SessionUpdate
	switch msg.Role {
	case fantasy.MessageRoleUser:
		for _, part := range msg.Content {
			switch p := part.(type) {
			case fantasy.TextPart:
				updates = append(updates, acp.UpdateUserMessage(acp.TextBlock(p.Text)))
			case fantasy.FilePart:
				updates = append(updates, acp.UpdateUserMessage(acp.ImageBlock(string(p.Data), p.MediaType)))
			}
		}
	case fantasy.MessageRoleAssistant:
		for _, part := range msg.Content {
			switch p := part.(type) {
			case fantasy.TextPart:
				updates = append(updates, acp.UpdateAgentMessage(acp.TextBlock(p.Text)))
			case fantasy.ReasoningPart:
				updates = append(updates, acp.UpdateAgentThought(acp.TextBlock(p.Text)))
			case fantasy.ToolCallPart:
				u := acp.UpdateToolCallStart(p.ToolCallID, p.ToolName)
				u.ToolCall.RawInput = json.RawMessage(p.Input)
				updates = append(updates, u)
			}
		}
	case fantasy.MessageRoleTool:
		for _, part := range msg.Content {
			if p, ok := part.(fantasy.ToolResultPart); ok {
				u := acp.UpdateToolCallDelta(p.ToolCallID)
				switch r := p.Output.(type) {
				case fantasy.ToolResultOutputContentText:
					u.ToolCallUpdate.RawOutput = r.Text
					status := acp.ToolCompleted
					u.ToolCallUpdate.Status = &status
				case fantasy.ToolResultOutputContentError:
					u.ToolCallUpdate.RawOutput = r.Error.Error()
					status := acp.ToolFailed
					u.ToolCallUpdate.Status = &status
				case fantasy.ToolResultOutputContentMedia:
					u.ToolCallUpdate.RawOutput = r.Text
					status := acp.ToolCompleted
					u.ToolCallUpdate.Status = &status
				}
				updates = append(updates, u)
			}
		}
	}
	return updates
}

// usageToACP maps Fantasy Usage to ACP Usage.
func UsageToACP(u fantasy.Usage) *acp.Usage {
	return &acp.Usage{
		InputTokens:       uint64(u.InputTokens),
		OutputTokens:      uint64(u.OutputTokens),
		TotalTokens:       uint64(u.TotalTokens),
		ThoughtTokens:     new(uint64(u.ReasoningTokens)),
		CachedReadTokens:  new(uint64(u.CacheReadTokens)),
		CachedWriteTokens: new(uint64(u.CacheCreationTokens)),
	}
}

func catwalkTypeToACP(t catwalk.Type) acp.LlmProtocol {
	switch t {
	case catwalk.TypeAnthropic:
		return acp.LlmProtocolAnthropic
	case catwalk.TypeGoogle:
		return acp.LlmProtocolGoogle
	case catwalk.TypeAzure:
		return acp.LlmProtocolAzure
	case catwalk.TypeBedrock:
		return acp.LlmProtocolBedrock
	case catwalk.TypeVertexAI:
		return acp.LlmProtocolVertex
	case catwalk.TypeOpenRouter:
		return acp.LlmProtocolOpenRouter
	case catwalk.TypeOpenAI:
		return acp.LlmProtocolOpenAI
	default:
		return acp.LlmProtocolOpenAICompat
	}
}
