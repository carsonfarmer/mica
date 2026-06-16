package core

import (
	"charm.land/catwalk/pkg/catwalk"
	acp "github.com/carsonfarmer/go-acp-sdk"
)

// ComputeCost calculates the cost for a single turn's usage against the model's pricing.
func ComputeCost(cfg catwalk.Model, usage *acp.Usage) *acp.Cost {
	if usage == nil {
		return nil
	}
	var total float64
	if cfg.CostPer1MIn > 0 && usage.InputTokens > 0 {
		total += float64(usage.InputTokens) * cfg.CostPer1MIn / 1e6
	}
	if cfg.CostPer1MOut > 0 && usage.OutputTokens > 0 {
		total += float64(usage.OutputTokens) * cfg.CostPer1MOut / 1e6
	}
	if cfg.CostPer1MInCached > 0 && usage.CachedReadTokens != nil && *usage.CachedReadTokens > 0 {
		total += float64(*usage.CachedReadTokens) * cfg.CostPer1MInCached / 1e6
	}
	if cfg.CostPer1MOutCached > 0 && usage.CachedWriteTokens != nil && *usage.CachedWriteTokens > 0 {
		total += float64(*usage.CachedWriteTokens) * cfg.CostPer1MOutCached / 1e6
	}
	if total == 0 {
		return nil
	}
	return &acp.Cost{Amount: total, Currency: "USD"}
}

// AccumulateUsage merges turn usage into the session's running totals.
// InputTokens from the turn already represent the full prompt (all history),
// so we take the latest value rather than summing. OutputTokens, reasoning,
// and cache tokens represent new work each turn and are accumulated.
func AccumulateUsage(total *acp.Usage, turn *acp.Usage) {
	total.InputTokens = turn.InputTokens
	total.OutputTokens += turn.OutputTokens
	if turn.ThoughtTokens != nil {
		if total.ThoughtTokens == nil {
			total.ThoughtTokens = new(uint64)
		}
		*total.ThoughtTokens += *turn.ThoughtTokens
	}
	if turn.CachedReadTokens != nil {
		if total.CachedReadTokens == nil {
			total.CachedReadTokens = new(uint64)
		}
		*total.CachedReadTokens += *turn.CachedReadTokens
	}
	if turn.CachedWriteTokens != nil {
		if total.CachedWriteTokens == nil {
			total.CachedWriteTokens = new(uint64)
		}
		*total.CachedWriteTokens += *turn.CachedWriteTokens
	}
	total.TotalTokens = total.InputTokens + total.OutputTokens
	if total.ThoughtTokens != nil {
		total.TotalTokens += *total.ThoughtTokens
	}
	if total.CachedReadTokens != nil {
		total.TotalTokens += *total.CachedReadTokens
	}
	if total.CachedWriteTokens != nil {
		total.TotalTokens += *total.CachedWriteTokens
	}
}
