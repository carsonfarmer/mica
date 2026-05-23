package agent

import (
	"context"
	"encoding/json"
	"time"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/mica/pkg/llm"
)

func (a *Agent) Prompt(ctx context.Context, req *acp.PromptRequest, client acp.Client) (*acp.PromptResponse, error) {
	sess, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "session not found")
	}
	if sess.Model == "" {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "no model configured")
	}

	var canceller agentutil.SessionCanceller
	a.mu.Lock()
	a.cancellers[req.SessionID] = &canceller
	a.mu.Unlock()

	ctx = canceller.Begin(ctx)
	defer func() {
		canceller.End()
		a.mu.Lock()
		delete(a.cancellers, req.SessionID)
		a.mu.Unlock()
	}()

	ctx = llm.WithClient(ctx, client)
	ctx = llm.WithSession(ctx, req.SessionID)

	model, err := a.reg.Resolve(ctx, sess.Model)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	userMsgID := req.MessageID
	if userMsgID == "" {
		userMsgID = acp.NewUUID()
	}

	userMsg, ok := llm.PromptToMessage(req.Prompt)
	if !ok {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "empty prompt")
	}

	prevEvents, _ := a.store.Load(ctx, req.SessionID)
	history := llm.HistoryToMessages(prevEvents)
	history = append(history, userMsg)

	stream := agentutil.NewSessionStream(a.bc, req.SessionID)

	fa := fantasy.NewAgent(model, fantasy.WithTools(a.tools...))

	call := fantasy.AgentStreamCall{
		Messages: history,
		ProviderOptions: fantasy.ProviderOptions{
			openaicompat.TypeProviderOptions: &openaicompat.ProviderOptions{
				ExtraBody: map[string]any{"reasoning": true},
			},
		},
		OnTextDelta: func(id, text string) error {
			return stream.SendText(ctx, text, id)
		},
		OnReasoningDelta: func(id, text string) error {
			return stream.SendThought(ctx, text, id)
		},
		OnToolCall: func(tc fantasy.ToolCallContent) error {
			return stream.StartToolCall(ctx, tc.ToolCallID, tc.ToolName, llm.ToolNameToACP(tc.ToolName))
		},
		OnToolResult: func(tr fantasy.ToolResultContent) error {
			switch tr.Result.(type) {
			case fantasy.ToolResultOutputContentError:
				return stream.FailToolCall(ctx, tr.ToolCallID)
			default:
				raw, _ := json.Marshal(tr.Result)
				return stream.CompleteToolCall(ctx, tr.ToolCallID, acp.ToolContent(acp.TextBlock(string(raw))))
			}
		},
	}

	result, err := fa.Stream(ctx, call)
	if err != nil {
		if ctx.Err() != nil {
			return &acp.PromptResponse{StopReason: acp.StopCancelled, UserMessageID: userMsgID}, nil
		}
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	// Persist user updates and results.
	head, err := a.store.Head(ctx, req.SessionID)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	for _, upd := range llm.MessageToACP(userMsg) {
		upd.UserMessageChunk.MessageID = userMsgID
		head, err = a.store.Append(ctx, req.SessionID, upd, head)
		if err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
	}

	for _, step := range result.Steps {
		for _, msg := range step.Messages {
			for _, upd := range llm.MessageToACP(msg) {
				head, err = a.store.Append(ctx, req.SessionID, upd, head)
				if err != nil {
					return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
				}
				stream.SendUpdate(ctx, upd)
			}
		}
	}

	stopReason := llm.FinishReasonToACP(result.Response.FinishReason)
	usage := llm.UsageToACP(result.Response.Usage)

	if len(req.Prompt) > 0 {
		sess.SessionInfo.Title = acp.TitleFromPrompt(req.Prompt)
	}
	if sess.SessionInfo.Title != "" {
		upd := acp.UpdateSessionInfo(sess.SessionInfo.Title, time.Now())
		head, err = a.store.Append(ctx, req.SessionID, upd, head)
		if err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
		stream.SendUpdate(ctx, upd)
	}

	if usage != nil {
		upd := acp.UpdateUsage(usage.TotalTokens, 0, nil)
		head, err = a.store.Append(ctx, req.SessionID, upd, head)
		if err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
		stream.SendUpdate(ctx, upd)
	}

	if head != nil {
		a.store.Commit(ctx, req.SessionID, *head)
	}

	return &acp.PromptResponse{
		StopReason:    stopReason,
		Usage:         usage,
		UserMessageID: userMsgID,
	}, nil
}
