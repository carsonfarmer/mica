package core

import (
	"context"
	"strings"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	"github.com/carsonfarmer/go-acp-sdk"
)


func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
	// Empty registry should have no providers.
	if len(reg.Providers()) != 0 {
		t.Fatal("expected no providers in empty registry")
	}
}

func TestAddProviderCatalog(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "mockai",
		Name:                "MockAI",
		Type:                catwalk.TypeOpenAI,
		APIEndpoint:         "https://api.mockai.example/v1",
		DefaultLargeModelID: "mock-fast",
		Models: []catwalk.Model{
			{
				ID:            "mock-fast",
				Name:          "Mock Fast",
				ContextWindow: 64_000,
				CostPer1MIn:  2.00,
				CostPer1MOut:  8.00,
			},
			{
				ID:                  "mock-slow",
				Name:                "Mock Slow",
				ContextWindow:       128_000,
				CanReason:           true,
				ReasoningLevels:     []string{"low", "high"},
				DefaultReasoningEffort: "low",
			},
		},
}, nil)

	// Config
	cfg, ok := reg.Config(FullModelID("mockai/mock-fast"))
	if !ok {
		t.Fatal("expected to find mock-fast")
	}
	if cfg.Name != "Mock Fast" {
		t.Fatalf("Name = %q", cfg.Name)
	}

	_, ok = reg.Config(FullModelID("mockai/nonexistent"))
	if ok {
		t.Fatal("expected not found")
	}
	_, ok = reg.Config(FullModelID("noprovider/mock-fast"))
	if ok {
		t.Fatal("expected not found")
	}

	// Default
	def := reg.Default()
	prov, model, _ := strings.Cut(string(def), "/")
	if prov != "mockai" || model != "mock-fast" {
		t.Fatalf("Default = %s, want mockai/mock-fast", def)
	}

	// Models
	models := reg.Models("mockai")
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	models = reg.Models("noprovider")
	if models != nil {
		t.Fatal("expected nil for unknown provider")
	}

	// Providers
	providers := reg.Providers()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].ID != "mockai" {
		t.Fatalf("ID = %q", providers[0].ID)
	}

	// ProviderOptions
	opts := reg.ProviderOptions(FullModelID("mockai/mock-slow"), "high")
	if opts == nil {
		t.Fatal("expected non-nil options for reasoning model")
	}
	opts = reg.ProviderOptions(FullModelID("mockai/mock-fast"), "low")
	// ProviderOptions doesn't gate on CanReason — the caller does.
	// For an OpenAI-type provider, any thought level returns options.
	if opts == nil {
		t.Fatal("expected non-nil options for OpenAI provider with thought level")
	}
	opts = reg.ProviderOptions(FullModelID("mockai/mock-slow"), "")
	if opts != nil {
		t.Fatal("expected nil options for empty thought level")
	}
	opts = reg.ProviderOptions(FullModelID("noprovider/x"), "low")
	if opts != nil {
		t.Fatal("expected nil options for unknown provider")
	}
}

func TestResolveFailsWithoutRealProvider(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "mockai",
		DefaultLargeModelID: "mock-fast",
		Models:              []catwalk.Model{{ID: "mock-fast"}},
}, nil)
	// Resolve with a nil fantasy provider should return an error.
	_, err := reg.Resolve(context.Background(), FullModelID("mockai/mock-fast"))
	if err == nil {
		t.Fatal("expected error when resolving with nil fantasy provider")
	}
}

func TestModelsDescription(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "a",
		DefaultLargeModelID: "m1",
		Models: []catwalk.Model{
			{ID: "m1", Name: "Model 1", ContextWindow: 128_000},
			{ID: "m2", Name: "Model 2", ContextWindow: 0}, // no description
		},
}, nil)
	models := reg.Models("a")
	if len(models) != 2 {
		t.Fatal("expected 2 models")
	}
	if models[0].Description != "0.1M ctx" {
		t.Fatalf("Description = %q, want '0.1M ctx'", models[0].Description)
	}
	if models[1].Description != "" {
		t.Fatalf("Description = %q, want empty", models[1].Description)
	}
}

func TestProviderOptionsAnthropic(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "anth",
		Type:                catwalk.TypeAnthropic,
		DefaultLargeModelID: "claude",
		Models: []catwalk.Model{
			{ID: "claude", CanReason: true, ReasoningLevels: []string{"low", "high"}},
		},
}, nil)
	opts := reg.ProviderOptions(FullModelID("anth/claude"), "high")
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestProviderOptionsGoogle(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "google",
		Type:                catwalk.TypeGoogle,
		DefaultLargeModelID: "gemini",
		Models: []catwalk.Model{
			{ID: "gemini", CanReason: true, ReasoningLevels: []string{"low"}},
		},
}, nil)
	opts := reg.ProviderOptions(FullModelID("google/gemini"), "low")
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestProviderOptionsOpenRouter(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "or",
		Type:                catwalk.TypeOpenRouter,
		DefaultLargeModelID: "sonnet",
		Models: []catwalk.Model{
			{ID: "sonnet", CanReason: true, ReasoningLevels: []string{"low", "medium", "high"}},
		},
}, nil)
	opts := reg.ProviderOptions(FullModelID("or/sonnet"), "medium")
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestProviderOptionsOpenAICompat(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "compat",
		Type:                catwalk.TypeOpenAICompat,
		DefaultLargeModelID: "model",
		Models: []catwalk.Model{
			{ID: "model", CanReason: true, ReasoningLevels: []string{"low"}},
		},
}, nil)
	opts := reg.ProviderOptions(FullModelID("compat/model"), "low")
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
}

func TestProviderOptionsUnsupportedType(t *testing.T) {
	reg := NewRegistry()
	reg.addEntry(catwalk.Provider{
		ID:                  "bedrock",
		Type:                catwalk.TypeBedrock,
		DefaultLargeModelID: "model",
		Models:              []catwalk.Model{{ID: "model"}},
}, nil)
	opts := reg.ProviderOptions(FullModelID("bedrock/model"), "low")
	if opts != nil {
		t.Fatal("expected nil options for unsupported type")
	}
}

func TestAddProviderMissingAPIKey(t *testing.T) {
	reg := NewRegistry()
	err := reg.AddProvider(catwalk.Provider{
		ID:                  "noapikey",
		Type:                catwalk.TypeOpenAI,
		APIEndpoint:         "https://api.example.com/v1",
		DefaultLargeModelID: "model",
		Models:              []catwalk.Model{{ID: "model"}},
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestModelsContextWindowDisplay(t *testing.T) {
	// covered by TestModelsDescription above; this prevents unused import
	_ = acp.ModelInfo{}
}


