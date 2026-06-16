package tools

import (
	"context"
	"fmt"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

// PlanInput is the input for the plan tool.
type PlanInput struct {
	Entries []acp.PlanEntry `json:"entries" description:"Ordered list of plan steps."`
}

// PlanTool creates a plan tool for the model to declare its approach.
func PlanTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		"plan",
		"Declare an ordered plan for multi-step tasks. Always use this for work involving more than a single step.",
		func(ctx context.Context, in PlanInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var completed, inProgress int
			for _, e := range in.Entries {
				switch e.Status {
				case acp.PlanCompleted:
					completed++
				case acp.PlanInProgress:
					inProgress++
				}
			}
			msg := fmt.Sprintf("%d task(s): %d completed, %d in progress", len(in.Entries), completed, inProgress)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithTitle("plan"),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithRawOutput(msg),
				acp.WithRawInput(in),
				acp.WithToolContent(acp.ToolContent(acp.TextBlock(msg))),
			)
			return ToolResponse(msg, upd, acp.UpdatePlan(in.Entries...)), nil
		},
	)
}
