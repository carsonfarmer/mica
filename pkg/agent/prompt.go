package agent

import (
	"context"
	"fmt"
	"time"

	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/mica/pkg/llm"
)

func (a *Agent) Prompt(ctx context.Context, req *acp.PromptRequest, client acp.Client) (*acp.PromptResponse, error) {
	sess, head, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "session not found")
	}

	ctx = a.cancellers.Begin(ctx, req.SessionID)
	defer a.cancellers.End(req.SessionID)

	ctx = llm.WithClient(ctx, client)
	ctx = llm.WithSession(ctx, req.SessionID)

	model, err := a.reg.Resolve(ctx, sess.Model)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	cfg, _ := a.reg.Config(sess.Model)

	userMsgID := req.MessageID
	if userMsgID == "" {
		userMsgID = acp.NewUUID()
	}

	userMsg, ok := llm.PromptToMessage(req.Prompt)
	if !ok {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "empty prompt")
	}

	prevEvents, err := a.store.Load(ctx, req.SessionID)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	history := llm.UpdatesToMessages(prevEvents)
	history = append(history, userMsg)

	// Persist user message directly from the prompt blocks.
	for _, b := range req.Prompt {
		upd := acp.UpdateUserMessage(b, userMsgID)
		if head, err = a.store.Append(ctx, req.SessionID, upd, head); err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
	}

	stream := agentutil.NewSessionStream(a.bc, req.SessionID)

	agentOpts := []fantasy.AgentOption{
		fantasy.WithSystemPrompt(fmt.Sprintf(SystemPrompt, req.SessionID, sess.CWD)),
		fantasy.WithTools(a.tools...),
	}
	if cfg.DefaultMaxTokens > 0 {
		agentOpts = append(agentOpts, fantasy.WithMaxOutputTokens(cfg.DefaultMaxTokens))
	}
	fa := fantasy.NewAgent(model, agentOpts...)

	providerOpts := a.reg.ProviderOptions(sess.Model, sess.ThoughtLevel)

	call := fantasy.AgentStreamCall{
		Messages:        history,
		ProviderOptions: providerOpts,
		OnTextDelta: func(id, text string) error {
			return stream.SendText(ctx, text, id)
		},
		OnReasoningDelta: func(id, text string) error {
			return stream.SendThought(ctx, text, id)
		},
		OnToolInputStart: func(id, toolName string) error {
			kind := llm.ToolNameToACP(toolName)
			return stream.StartToolCall(ctx, acp.ToolCallID(id), toolName, kind)
		},
		OnToolCall: func(tc fantasy.ToolCallContent) error {
			status := acp.ToolInProgress
			update := acp.UpdateToolCallDelta(tc.ToolCallID)
			update.ToolCallUpdate.Title = tc.ToolName
			update.ToolCallUpdate.Status = &status
			return stream.SendUpdate(ctx, update)
		},
		OnToolResult: func(tr fantasy.ToolResultContent) error {
			return stream.SendUpdate(ctx, llm.ToolResultToACP(tr))
		},
	}

	result, err := fa.Stream(ctx, call)
	if err != nil {
		if ctx.Err() != nil {
			return &acp.PromptResponse{StopReason: acp.StopCancelled, UserMessageID: userMsgID}, nil
		}
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	for _, step := range result.Steps {
		for _, upd := range llm.StepToACP(step.Messages, sess.CWD) {
			if head, err = a.store.Append(ctx, req.SessionID, upd, head); err != nil {
				return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
			}
		}
	}

	stopReason := llm.FinishReasonToACP(result.Response.FinishReason)
	usage := llm.UsageToACP(result.Response.Usage)

	if len(req.Prompt) > 0 {
		sess.SessionInfo.Title = agentutil.TitleFromPrompt(req.Prompt)
	}
	if sess.SessionInfo.Title != "" {
		a.store.Set(ctx, req.SessionID, sess)
		upd := acp.UpdateSessionInfo(sess.SessionInfo.Title, time.Now())
		if head, err = a.store.Append(ctx, req.SessionID, upd, head); err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
		stream.SendSessionInfo(ctx, sess.SessionInfo.Title)
	}

	if usage != nil {
		turnCost := computeCost(cfg, usage)

		// Accumulate into session totals.
		accumulateUsage(&sess.TotalUsage, usage)
		if turnCost != nil {
			if sess.TotalCost == nil {
				sess.TotalCost = &acp.Cost{Currency: "USD"}
			}
			sess.TotalCost.Amount += turnCost.Amount
		}
		a.store.Set(ctx, req.SessionID, sess)

		// Send cumulative usage to client.
		size := uint64(cfg.ContextWindow)
		upd := acp.UpdateUsage(sess.TotalUsage.TotalTokens, size, sess.TotalCost)
		if head, err = a.store.Append(ctx, req.SessionID, upd, head); err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
		stream.SendUsageUpdate(ctx, sess.TotalUsage.TotalTokens, size, sess.TotalCost)
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
