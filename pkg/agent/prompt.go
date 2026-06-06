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
	"github.com/carsonfarmer/mica/pkg/llm"
)

func (a *Agent) Prompt(ctx context.Context, req *acp.PromptRequest, client acp.Client) (*acp.PromptResponse, error) {
	sess, head, err := a.store.Get(ctx, req.SessionID)
	if err != nil {
		return nil, acp.NewRPCError(acp.ErrInvalidParams, "session not found")
	}

	ctx, token := a.cancellers.Begin(ctx, req.SessionID)
	defer token.Done()

	ctx = llm.WithClient(ctx, client)
	ctx = llm.WithSession(ctx, req.SessionID)
	ctx = llm.WithCWD(ctx, sess.CWD)

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

	for _, b := range req.Prompt {
		upd := acp.UpdateUserMessage(b, acp.WithMessageID(userMsgID))
		if head, err = a.store.Append(ctx, req.SessionID, upd, head); err != nil {
			return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
		}
	}

	stream := agentutil.NewSessionStream(a.bc, req.SessionID)
	ctx = llm.WithStream(ctx, stream)

	agentOpts := []fantasy.AgentOption{
		fantasy.WithSystemPrompt(fmt.Sprintf(SystemPrompt, req.SessionID, sess.CWD)),
		fantasy.WithTools(a.tools...),
	}
	if cfg.DefaultMaxTokens > 0 {
		agentOpts = append(agentOpts, fantasy.WithMaxOutputTokens(cfg.DefaultMaxTokens))
	}
	fa := fantasy.NewAgent(model, agentOpts...)

	providerOpts := a.reg.ProviderOptions(sess.Model, sess.ThoughtLevel)

	var textBufs = map[string]*strings.Builder{}

	call := fantasy.AgentStreamCall{
		Messages:        history,
		ProviderOptions: providerOpts,

		OnTextDelta: func(id, text string) error {
			b, ok := textBufs[id]
			if !ok {
				b = &strings.Builder{}
				textBufs[id] = b
			}
			b.WriteString(text)
			return stream.SendText(ctx, text)
		},
		OnTextEnd: func(id string) error {
			buf, ok := textBufs[id]
			if !ok {
				return nil
			}
			delete(textBufs, id)
			upd := acp.UpdateAgentMessage(acp.TextBlock(buf.String()), acp.WithMessageID(id))
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
			kind := llm.ToolNameToACP(tc.ToolName)
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
			return &acp.PromptResponse{StopReason: acp.StopCancelled, UserMessageID: userMsgID}, nil
		}
		return nil, acp.NewRPCError(acp.ErrInternal, err.Error())
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

		accumulateUsage(&sess.TotalUsage, usage)
		if turnCost != nil {
			if sess.TotalCost == nil {
				sess.TotalCost = &acp.Cost{Currency: "USD"}
			}
			sess.TotalCost.Amount += turnCost.Amount
		}
		a.store.Set(ctx, req.SessionID, sess)

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
