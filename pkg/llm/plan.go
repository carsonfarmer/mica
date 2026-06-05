package llm

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
)

const ToolNamePlan = "plan"

// PlanInput is the input for the plan tool.
type PlanInput struct {
	Entries []acp.PlanEntry `json:"entries" description:"Ordered list of plan steps."`
}

// PlanTool creates a plan tool for the model to declare its approach.
func PlanTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		ToolNamePlan,
		"Declare an ordered plan for multi-step tasks. Always use this for work involving more than a single step.",
		func(ctx context.Context, in PlanInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			stream := agentutil.NewSessionStream(ClientFrom(ctx), SessionFrom(ctx))
			if err := stream.SendPlan(ctx, in.Entries); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID))
			var b strings.Builder
			for _, e := range in.Entries {
				s := " "
				if e.Status == "completed" {
					s = "x"
				}
				fmt.Fprintf(&b, "- [%s] (*%s*) %s  **(%s)**\n", s, e.Status, e.Content, e.Priority)
			}
			upd.ToolCallUpdate.RawInput = in
			upd.ToolCallUpdate.Content = []acp.ToolCallContent{acp.ToolContent(acp.TextBlock(b.String()))}
			stream.SendUpdate(ctx, upd)

			return fantasy.NewTextResponse("plan updated"), nil
		},
	)
}
