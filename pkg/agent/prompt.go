package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/mica/pkg/core"
)

func (a *Agent) Prompt(ctx context.Context, req *acp.PromptRequest, client acp.Client) (*acp.PromptResponse, error) {
	sess, head, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "session not found")
	}

	ctx, token := a.cancellers.Begin(ctx, req.SessionID)
	defer token.Done()

	ctx = core.WithClient(ctx, client)
	ctx = core.WithSession(ctx, sess)

	model, err := a.reg.Resolve(ctx, sess.Model)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	cfg, _ := a.reg.Config(sess.Model)

	userMessageID := req.MessageID
	if userMessageID == "" {
		userMessageID = acp.NewUUID()
	}

	userMsg, ok := core.PromptToMessage(req.Prompt)
	if !ok {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "empty prompt")
	}

	prevEvents, err := a.store.Load(ctx, req.SessionID)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}
	history := core.UpdatesToMessages(prevEvents)
	history = append(history, userMsg)

	for _, b := range req.Prompt {
		upd := acp.UpdateUserMessage(b, acp.WithMessageID(userMessageID))
		if head, err = a.store.Append(ctx, req.SessionID, upd, head); err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
		if err := a.bc.SendExcept(ctx, &acp.SessionNotification{
			SessionID: req.SessionID,
			Update:    upd,
		}, client); err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
	}

	stream := agentutil.NewSessionStream(a.bc, req.SessionID)

	var sysPrompt strings.Builder
	fmt.Fprintf(&sysPrompt, SystemPrompt, req.SessionID, sess.SessionInfo.CWD)
	for _, h := range sess.PromptHooks {
		h(&sysPrompt)
	}

	agentOpts := []fantasy.AgentOption{
		fantasy.WithSystemPrompt(sysPrompt.String()),
		fantasy.WithTools(a.tools...),
	}
	if cfg.DefaultMaxTokens > 0 {
		agentOpts = append(agentOpts, fantasy.WithMaxOutputTokens(cfg.DefaultMaxTokens))
	}
	fa := fantasy.NewAgent(model, agentOpts...)

	providerOpts := a.reg.ProviderOptions(sess.Model, sess.ThoughtLevel)

	var textBuf strings.Builder

	call := fantasy.AgentStreamCall{
		Messages:        history,
		ProviderOptions: providerOpts,

		OnTextDelta: func(id, text string) error {
			textBuf.WriteString(text)
			return stream.SendText(ctx, text)
		},
		OnTextEnd: func(id string) error {
			s := textBuf.String()
			textBuf.Reset()
			upd := acp.UpdateAgentMessage(acp.TextBlock(s), acp.WithMessageID(id))
			head, err = a.store.Append(ctx, req.SessionID, upd, head)
			return err
		},
		OnReasoningDelta: func(id, text string) error {
			return stream.SendThought(ctx, text)
		},
		OnReasoningEnd: func(id string, rc fantasy.ReasoningContent) error {
			upd := acp.UpdateAgentThought(acp.TextBlock(rc.Text), acp.WithMessageID(id))
			head, err = a.store.Append(ctx, req.SessionID, upd, head)
			return err
		},
		OnToolCall: func(tc fantasy.ToolCallContent) error {
			kind := core.ToolNameToACP(tc.ToolName)
			upd := acp.UpdateToolCallStart(
				acp.ToolCallID(tc.ToolCallID),
				acp.WithTitle(tc.ToolName),
				acp.WithKind(kind),
				acp.WithRawInput(json.RawMessage(tc.Input)),
			)
			if err := stream.SendUpdate(ctx, upd); err != nil {
				return err
			}
			head, err = a.store.Append(ctx, req.SessionID, upd, head)
			return err
		},
		OnToolResult: func(tr fantasy.ToolResultContent) error {
			if tr.ClientMetadata == "" {
				return nil
			}
			var updates []acp.SessionUpdate
			if err := json.Unmarshal([]byte(tr.ClientMetadata), &updates); err != nil {
				return err
			}
			for _, upd := range updates {
				if err := stream.SendUpdate(ctx, upd); err != nil {
					return err
				}
				head, err = a.store.Append(ctx, req.SessionID, upd, head)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}

	result, err := fa.Stream(ctx, call)
	if err != nil {
		if ctx.Err() != nil {
			return &acp.PromptResponse{StopReason: acp.StopCancelled, UserMessageID: userMessageID}, nil
		}
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
	}

	stopReason := core.FinishReasonToACP(result.Response.FinishReason)
	usage := core.UsageToACP(result.Response.Usage)

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
		turnCost := core.ComputeCost(cfg, usage)

		core.AccumulateUsage(&sess.Usage, usage)
		if turnCost != nil {
			if sess.Cost == nil {
				sess.Cost = &acp.Cost{Currency: "USD"}
			}
			sess.Cost.Amount += turnCost.Amount
		}
		a.store.Set(ctx, req.SessionID, sess)

		size := uint64(cfg.ContextWindow)
		upd := acp.UpdateUsage(sess.Usage.TotalTokens, size, sess.Cost)
		if head, err = a.store.Append(ctx, req.SessionID, upd, head); err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
		stream.SendUsageUpdate(ctx, sess.Usage.TotalTokens, size, sess.Cost)
	}

	if head != nil {
		a.store.Commit(ctx, req.SessionID, *head)
	}

	return &acp.PromptResponse{
		StopReason:    stopReason,
		Usage:         usage,
		UserMessageID: userMessageID,
	}, nil
}
