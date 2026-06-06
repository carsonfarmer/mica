package llm

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
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
			var b strings.Builder
			for _, e := range in.Entries {
				s := " "
				if e.Status == "completed" {
					s = "x"
				}
				fmt.Fprintf(&b, "- [%s] (*%s*) %s  **(%s)**\n", s, e.Status, e.Content, e.Priority)
			}

			planUpd := acp.UpdatePlan(in.Entries...)
			toolUpd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithTitle(ToolNamePlan),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithRawOutput("plan updated"),
				acp.WithRawInput(in),
				acp.WithToolContent(acp.ToolContent(acp.TextBlock(b.String()))),
			)

			return ToolResponse("plan updated", planUpd, toolUpd), nil
		},
	)
}
