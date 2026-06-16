package core

import (
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/carsonfarmer/go-acp-sdk"
)

func TestComputeCostNilUsage(t *testing.T) {
	cost := ComputeCost(catwalk.Model{}, nil)
	if cost != nil {
		t.Fatal("expected nil cost for nil usage")
	}
}

func TestComputeCostZeroRates(t *testing.T) {
	usage := &acp.Usage{InputTokens: 1000, OutputTokens: 500}
	cost := ComputeCost(catwalk.Model{}, usage)
	if cost != nil {
		t.Fatal("expected nil cost when rates are zero")
	}
}

func TestComputeCostInputOnly(t *testing.T) {
	usage := &acp.Usage{InputTokens: 1_000_000}
	cfg := catwalk.Model{CostPer1MIn: 3.00}
	cost := ComputeCost(cfg, usage)
	if cost == nil {
		t.Fatal("expected non-nil cost")
	}
	if cost.Amount != 3.00 {
		t.Fatalf("expected $3.00, got $%.2f", cost.Amount)
	}
}

func TestComputeCostOutputOnly(t *testing.T) {
	usage := &acp.Usage{OutputTokens: 500_000}
	cfg := catwalk.Model{CostPer1MOut: 15.00}
	cost := ComputeCost(cfg, usage)
	if cost == nil {
		t.Fatal("expected non-nil cost")
	}
	if cost.Amount != 7.50 {
		t.Fatalf("expected $7.50, got $%.2f", cost.Amount)
	}
}

func TestComputeCostCached(t *testing.T) {
	cr := uint64(2_000_000)
	cw := uint64(500_000)
	usage := &acp.Usage{
		InputTokens:       1_000_000,
		OutputTokens:      1_000_000,
		CachedReadTokens:  &cr,
		CachedWriteTokens: &cw,
	}
	cfg := catwalk.Model{
		CostPer1MIn:       3.00,
		CostPer1MOut:      15.00,
		CostPer1MInCached: 1.25,
		CostPer1MOutCached: 2.00,
	}
	cost := ComputeCost(cfg, usage)
	if cost == nil {
		t.Fatal("expected non-nil cost")
	}
	// 1M * 3 + 1M * 15 + 2M * 1.25 + 500k * 2.00
	// = 3 + 15 + 2.5 + 1.0 = 21.50
	if cost.Amount != 21.50 {
		t.Fatalf("expected $21.50, got $%.2f", cost.Amount)
	}
	if cost.Currency != "USD" {
		t.Fatalf("expected USD, got %s", cost.Currency)
	}
}

func TestAccumulateUsageFresh(t *testing.T) {
	turn := &acp.Usage{
		InputTokens:  1000,
		OutputTokens: 50,
		TotalTokens:  1050,
	}
	total := &acp.Usage{}
	AccumulateUsage(total, turn)

	// Input is set (not added) since it represents cumulative prompt.
	if total.InputTokens != 1000 {
		t.Fatalf("InputTokens = %d, want 1000", total.InputTokens)
	}
	if total.OutputTokens != 50 {
		t.Fatalf("OutputTokens = %d, want 50", total.OutputTokens)
	}
	if total.TotalTokens != 1050 {
		t.Fatalf("TotalTokens = %d, want 1050", total.TotalTokens)
	}
}

func TestAccumulateUsageMultiTurn(t *testing.T) {
	total := &acp.Usage{}

	// Turn 1: prompt is small, output is small
	AccumulateUsage(total, &acp.Usage{InputTokens: 100, OutputTokens: 50})
	if total.InputTokens != 100 || total.OutputTokens != 50 || total.TotalTokens != 150 {
		t.Fatalf("turn 1: got in=%d out=%d tot=%d", total.InputTokens, total.OutputTokens, total.TotalTokens)
	}

	// Turn 2: prompt grows (includes history), output adds
	AccumulateUsage(total, &acp.Usage{InputTokens: 250, OutputTokens: 100})
	if total.InputTokens != 250 || total.OutputTokens != 150 || total.TotalTokens != 400 {
		t.Fatalf("turn 2: got in=%d out=%d tot=%d", total.InputTokens, total.OutputTokens, total.TotalTokens)
	}

	// Turn 3: prompt grows again
	AccumulateUsage(total, &acp.Usage{InputTokens: 500, OutputTokens: 200})
	if total.InputTokens != 500 || total.OutputTokens != 350 || total.TotalTokens != 850 {
		t.Fatalf("turn 3: got in=%d out=%d tot=%d", total.InputTokens, total.OutputTokens, total.TotalTokens)
	}
}

func TestAccumulateUsageWithPointers(t *testing.T) {
	rt := uint64(20)
	cr := uint64(10)
	cw := uint64(5)
	turn := &acp.Usage{
		InputTokens:       200,
		OutputTokens:      50,
		TotalTokens:       250,
		ThoughtTokens:     &rt,
		CachedReadTokens:  &cr,
		CachedWriteTokens: &cw,
	}
	total := &acp.Usage{}
	AccumulateUsage(total, turn)
	AccumulateUsage(total, turn) // round 2: input is same cumulative value, output + reasoning adds

	// Input stays at latest turn value (not accumulated).
	if total.InputTokens != 200 {
		t.Fatalf("InputTokens = %d, want 200", total.InputTokens)
	}
	// Output accumulates: 50 + 50 = 100
	if total.OutputTokens != 100 {
		t.Fatalf("OutputTokens = %d, want 100", total.OutputTokens)
	}
	// Total = input(200) + output(100) + thought(40) + cacheRead(20) + cacheWrite(10) = 370
	if total.TotalTokens != 370 {
		t.Fatalf("TotalTokens = %d, want 370", total.TotalTokens)
	}
	if total.ThoughtTokens == nil || *total.ThoughtTokens != 40 {
		t.Fatalf("ThoughtTokens = %v, want 40", total.ThoughtTokens)
	}
	if total.CachedReadTokens == nil || *total.CachedReadTokens != 20 {
		t.Fatalf("CachedReadTokens = %v, want 20", total.CachedReadTokens)
	}
	if total.CachedWriteTokens == nil || *total.CachedWriteTokens != 10 {
		t.Fatalf("CachedWriteTokens = %v, want 10", total.CachedWriteTokens)
	}
}

func TestAccumulateUsageNilPointersInTurn(t *testing.T) {
	turn := &acp.Usage{
		InputTokens:  10,
		OutputTokens: 5,
		TotalTokens:  15,
	}
	total := &acp.Usage{}
	AccumulateUsage(total, turn)
	AccumulateUsage(total, turn)

	// pointer fields in turn were nil, so total pointer fields stay nil
	if total.ThoughtTokens != nil {
		t.Fatal("expected nil ThoughtTokens when turn had none")
	}
	if total.CachedReadTokens != nil {
		t.Fatal("expected nil CachedReadTokens when turn had none")
	}
}
