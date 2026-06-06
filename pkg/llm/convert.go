package llm

import (
	"encoding/json"
	"errors"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

// UpdatesToMessages converts ACP session updates into Fantasy messages
// suitable for LLM conversation history.
func UpdatesToMessages(updates []acp.SessionUpdate) []fantasy.Message {
	var msgs []fantasy.Message
	for _, u := range updates {
		switch {
		case u.UserMessageChunk != nil:
			if p := contentBlockToMessagePart(u.UserMessageChunk.Content); p != nil {
				msgs = append(msgs, fantasy.Message{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{p}})
			}
		case u.AgentMessageChunk != nil:
			if p := contentBlockToMessagePart(u.AgentMessageChunk.Content); p != nil {
				msgs = append(msgs, fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{p}})
			}
		case u.AgentThoughtChunk != nil:
			msgs = append(msgs, fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{fantasy.ReasoningPart{Text: u.AgentThoughtChunk.Content.Text.Text}},
			})
		case u.ToolCall != nil:
			if m := toolCallToMessage(u.ToolCall); m != nil {
				msgs = append(msgs, *m)
			}
		case u.ToolCallUpdate != nil:
			if m := toolCallUpdateToMessage(u.ToolCallUpdate); m != nil {
				msgs = append(msgs, *m)
			}
		}
	}
	return msgs
}

func toolCallToMessage(tc *acp.SessionUpdateToolCall) *fantasy.Message {
	// Title is the tool name, set by UpdateToolCallStart in OnToolCall.
	// Only ToolCallUpdate (deltas) modify Title for display purposes.
	if tc.Title == "" {
		return nil
	}
	input, _ := json.Marshal(tc.RawInput)
	return &fantasy.Message{
		Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{
			fantasy.ToolCallPart{ToolCallID: tc.ToolCallID, ToolName: tc.Title, Input: string(input)},
		},
	}
}

func toolCallUpdateToMessage(tu *acp.SessionUpdateToolCallUpdate) *fantasy.Message {
	s, ok := tu.RawOutput.(string)
	if !ok {
		return nil
	}
	result := fantasy.ToolResultOutputContent(fantasy.ToolResultOutputContentText{Text: s})
	if tu.Status != nil && *tu.Status == acp.ToolFailed {
		result = fantasy.ToolResultOutputContentError{Error: errors.New(s)}
	}
	return &fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{ToolCallID: tu.ToolCallID, Output: result},
		},
	}
}

// PromptToMessage converts ACP content blocks into a Fantasy user message.
func PromptToMessage(blocks []acp.ContentBlock) (fantasy.Message, bool) {
	parts := make([]fantasy.MessagePart, 0, len(blocks))
	for _, b := range blocks {
		if p := contentBlockToMessagePart(b); p != nil {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return fantasy.Message{}, false
	}
	return fantasy.Message{Role: fantasy.MessageRoleUser, Content: parts}, true
}

func contentBlockToMessagePart(b acp.ContentBlock) fantasy.MessagePart {
	switch {
	case b.Text != nil:
		return fantasy.TextPart{Text: b.Text.Text}
	case b.Image != nil:
		return fantasy.FilePart{
			Data:      []byte(b.Image.Data),
			MediaType: b.Image.MimeType,
		}
	case b.Resource != nil:
		switch {
		case b.Resource.Resource.Text != nil:
			return fantasy.FilePart{
				Data:      []byte(b.Resource.Resource.Text.Text),
				MediaType: b.Resource.Resource.Text.MimeType,
			}
		case b.Resource.Resource.Blob != nil:
			return fantasy.FilePart{
				Data:      []byte(b.Resource.Resource.Blob.Blob),
				MediaType: b.Resource.Resource.Blob.MimeType,
			}
		}
	case b.ResourceLink != nil:
		return fantasy.TextPart{Text: b.ResourceLink.URI}
	}
	return nil
}

// ToolNameToACP returns the ACP ToolKind for a registered tool name.
func ToolNameToACP(name string) acp.ToolKind {
	switch name {
	case ToolNameRead:
		return acp.ToolRead
	case ToolNameWrite, ToolNameEdit:
		return acp.ToolEdit
	case ToolNameExecute, ToolNameTerminalCreate, ToolNameTerminalOutput,
		ToolNameTerminalWait, ToolNameTerminalKill, ToolNameTerminalRelease:
		return acp.ToolExecute
	case ToolNamePlan:
		return acp.ToolThink
	default:
		return acp.ToolOther
	}
}

// FinishReasonToACP maps Fantasy finish reasons to ACP stop reasons.
func FinishReasonToACP(fr fantasy.FinishReason) acp.StopReason {
	switch fr {
	case fantasy.FinishReasonStop:
		return acp.StopEndTurn
	case fantasy.FinishReasonLength:
		return acp.StopMaxTokens
	case fantasy.FinishReasonToolCalls:
		return acp.StopEndTurn
	case fantasy.FinishReasonContentFilter:
		return acp.StopRefusal
	case fantasy.FinishReasonError:
		return acp.StopEndTurn
	default:
		return acp.StopEndTurn
	}
}

// TypeToACP maps a catwalk provider type to the ACP protocol enum.
func TypeToACP(t catwalk.Type) acp.LlmProtocol {
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

// UsageToACP maps Fantasy Usage to ACP Usage.
func UsageToACP(u fantasy.Usage) *acp.Usage {
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		return nil
	}
	return &acp.Usage{
		InputTokens:       uint64(u.InputTokens),
		OutputTokens:      uint64(u.OutputTokens),
		TotalTokens:       uint64(u.TotalTokens),
		ThoughtTokens:     new(uint64(u.ReasoningTokens)),
		CachedReadTokens:  new(uint64(u.CacheReadTokens)),
		CachedWriteTokens: new(uint64(u.CacheCreationTokens)),
	}
}
