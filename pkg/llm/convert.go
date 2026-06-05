package llm

import (
	"encoding/json"
	"fmt"

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
			if p := blockToPart(u.UserMessageChunk.Content); p != nil {
				msgs = append(msgs, fantasy.Message{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{p}})
			}
		case u.AgentMessageChunk != nil:
			if p := blockToPart(u.AgentMessageChunk.Content); p != nil {
				msgs = append(msgs, fantasy.Message{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{p}})
			}
		case u.AgentThoughtChunk != nil:
			msgs = append(msgs, fantasy.Message{
				Role:    fantasy.MessageRoleAssistant,
				Content: []fantasy.MessagePart{fantasy.ReasoningPart{Text: u.AgentThoughtChunk.Content.Text.Text}},
			})
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

func convertToolCall(tc *acp.SessionUpdateToolCall) *fantasy.Message {
	input, _ := json.Marshal(tc.RawInput)
	toolName := kindToName(tc)
	if toolName == "" {
		return nil
	}
	return &fantasy.Message{
		Role: fantasy.MessageRoleAssistant,
		Content: []fantasy.MessagePart{
			fantasy.ToolCallPart{ToolCallID: tc.ToolCallID, ToolName: toolName, Input: string(input)},
		},
	}
}

func kindToName(tc *acp.SessionUpdateToolCall) string {
	if tc.Kind == nil {
		return ""
	}
	switch *tc.Kind {
	case acp.ToolRead:
		return ToolNameReadFile
	case acp.ToolEdit:
		return editOrWrite(tc.RawInput)
	case acp.ToolExecute:
		return ToolNameExecuteCommand
	case acp.ToolThink:
		return ToolNamePlan
	default:
		return ""
	}
}

func editOrWrite(raw any) string {
	b, _ := json.Marshal(raw)
	var v struct{ Edits []Edit `json:"edits"` }
	if json.Unmarshal(b, &v) == nil && len(v.Edits) > 0 {
		return ToolNameEdit
	}
	return ToolNameWriteFile
}

func convertToolCallUpdate(tu *acp.SessionUpdateToolCallUpdate) *fantasy.Message {
	var result fantasy.ToolResultOutputContent
	switch {
	case tu.Status != nil && *tu.Status == acp.ToolFailed:
		result = fantasy.ToolResultOutputContentError{Error: fmt.Errorf("tool call failed")}
	case tu.RawOutput != nil:
		if s, ok := tu.RawOutput.(string); ok {
			result = fantasy.ToolResultOutputContentText{Text: s}
		}
	case len(tu.Content) > 0:
		result = contentToResult(tu.Content)
	}
	return &fantasy.Message{
		Role: fantasy.MessageRoleTool,
		Content: []fantasy.MessagePart{
			fantasy.ToolResultPart{ToolCallID: tu.ToolCallID, Output: result},
		},
	}
}

func contentToResult(blocks []acp.ToolCallContent) fantasy.ToolResultOutputContent {
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
	return fantasy.Message{Role: fantasy.MessageRoleUser, Content: parts}, true
}

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

// StepToACP converts a sequence of Fantasy messages into ACP session updates
// for persistence. Tool calls and results are stored with minimal metadata;
// rich display content is owned by the tool handlers during live streaming.
func StepToACP(msgs []fantasy.Message, _ string) []acp.SessionUpdate {
	var updates []acp.SessionUpdate

	for _, msg := range msgs {
		switch msg.Role {
		case fantasy.MessageRoleUser:
			for _, p := range msg.Content {
				switch p := p.(type) {
				case fantasy.TextPart:
					updates = append(updates, acp.UpdateUserMessage(acp.TextBlock(p.Text)))
				case fantasy.FilePart:
					updates = append(updates, acp.UpdateUserMessage(acp.ImageBlock(string(p.Data), p.MediaType)))
				}
			}
		case fantasy.MessageRoleAssistant:
			for _, p := range msg.Content {
				switch p := p.(type) {
				case fantasy.TextPart:
					updates = append(updates, acp.UpdateAgentMessage(acp.TextBlock(p.Text)))
				case fantasy.ReasoningPart:
					updates = append(updates, acp.UpdateAgentThought(acp.TextBlock(p.Text)))
				case fantasy.ToolCallPart:
					u := acp.UpdateToolCallStart(p.ToolCallID, p.ToolName)
					u.ToolCall.RawInput = json.RawMessage(p.Input)
					kind := ToolNameToACP(p.ToolName)
					u.ToolCall.Kind = &kind
					updates = append(updates, u)
				}
			}
		case fantasy.MessageRoleTool:
			for _, p := range msg.Content {
				p, ok := p.(fantasy.ToolResultPart)
				if !ok {
					continue
				}
				u := acp.UpdateToolCallDelta(acp.ToolCallID(p.ToolCallID))
				setResult(u.ToolCallUpdate, p.Output)
				updates = append(updates, u)
			}
		}
	}
	return updates
}

// ToolNameToACP returns the ACP ToolKind for a registered tool name.
func ToolNameToACP(name string) acp.ToolKind {
	switch name {
	case ToolNameReadFile:
		return acp.ToolRead
	case ToolNameWriteFile, ToolNameEdit:
		return acp.ToolEdit
	case ToolNameExecuteCommand, ToolNameTerminalCreate, ToolNameTerminalOutput,
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

// ToolResultToACP converts a Fantasy tool result into an ACP tool_call_update.
func ToolResultToACP(tr fantasy.ToolResultContent) acp.SessionUpdate {
	u := acp.UpdateToolCallDelta(tr.ToolCallID)
	setResult(u.ToolCallUpdate, tr.Result)
	return u
}

func setResult(u *acp.SessionUpdateToolCallUpdate, result fantasy.ToolResultOutputContent) {
	switch r := result.(type) {
	case fantasy.ToolResultOutputContentText:
		u.RawOutput = r.Text
		status := acp.ToolCompleted
		u.Status = &status
	case fantasy.ToolResultOutputContentError:
		u.RawOutput = r.Error.Error()
		status := acp.ToolFailed
		u.Status = &status
	case fantasy.ToolResultOutputContentMedia:
		u.RawOutput = r.Data
		status := acp.ToolCompleted
		u.Status = &status
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
