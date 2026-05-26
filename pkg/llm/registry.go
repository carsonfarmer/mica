package llm

import (
	"context"
	"fmt"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
)

type entry struct {
	catwalk catwalk.Provider
	fantasy fantasy.Provider
}

// Registry maps catwalk providers to fantasy providers and their models.
type Registry struct {
	providers map[catwalk.InferenceProvider]*entry
	default_  FullModelID
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[catwalk.InferenceProvider]*entry)}
}

// AddProvider creates a fantasy provider from the catwalk config and registers all models.
func (r *Registry) AddProvider(cw catwalk.Provider) error {
	prov, err := newFantasyProvider(cw)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	r.providers[cw.ID] = &entry{catwalk: cw, fantasy: prov}
	r.default_ = FullModelID{Provider: cw.ID, Model: cw.DefaultLargeModelID}
	return nil
}

// Resolve returns the LanguageModel for "provider/model".
func (r *Registry) Resolve(ctx context.Context, mid FullModelID) (fantasy.LanguageModel, error) {
	ent, ok := r.providers[mid.Provider]
	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q", mid.Provider)
	}
	return ent.fantasy.LanguageModel(ctx, mid.Model)
}

// Config returns the catwalk model config for "provider/model".
func (r *Registry) Config(mid FullModelID) (catwalk.Model, bool) {
	ent, ok := r.providers[mid.Provider]
	if !ok {
		return catwalk.Model{}, false
	}
	for _, m := range ent.catwalk.Models {
		if m.ID == mid.Model {
			return m, true
		}
	}
	return catwalk.Model{}, false
}

// Default returns the default model ID.
func (r *Registry) Default() FullModelID { return r.default_ }

// Providers returns the list of registered providers as ACP ProviderInfo.
func (r *Registry) Providers() []acp.ProviderInfo {
	var out []acp.ProviderInfo
	for _, ent := range r.providers {
		out = append(out, acp.ProviderInfo{
			ID:        string(ent.catwalk.Type),
			Supported: []acp.LlmProtocol{catwalkTypeToACP(ent.catwalk.Type)},
			Current: &acp.ProviderCurrentConfig{
				APIType: catwalkTypeToACP(ent.catwalk.Type),
				BaseURL: ent.catwalk.APIEndpoint,
			},
		})
	}
	return out
}

func catwalkTypeToACP(t catwalk.Type) acp.LlmProtocol {
	switch t {
	case catwalk.TypeAnthropic:
		return acp.LlmProtocolAnthropic
	case catwalk.TypeGoogle:
		return acp.LlmProtocolGoogle
	case catwalk.TypeAzure:
		return acp.LlmProtocolAzure
	case catwalk.TypeBedrock:
		return acp.LlmProtocolBedrock
	case catwalk.TypeVertexAI:
		return acp.LlmProtocolVertex
	case catwalk.TypeOpenRouter:
		return acp.LlmProtocolOpenRouter
	case catwalk.TypeOpenAI:
		return acp.LlmProtocolOpenAI
	default:
		return acp.LlmProtocolOpenAICompat
	}
}

// Models returns all models for a given provider as ACP ModelInfo.
func (r *Registry) Models(providerID string) []acp.ModelInfo {
	return r.modelsFor(catwalk.InferenceProvider(providerID))
}

func (r *Registry) modelsFor(id catwalk.InferenceProvider) []acp.ModelInfo {
	ent, ok := r.providers[id]
	if !ok {
		return nil
	}
	out := make([]acp.ModelInfo, 0, len(ent.catwalk.Models))
	for _, m := range ent.catwalk.Models {
		out = append(out, catwalkToModelInfo(string(id), m))
	}
	return out
}

func catwalkToModelInfo(provID string, m catwalk.Model) acp.ModelInfo {
	info := acp.ModelInfo{
		ModelID: acp.ModelID(fmt.Sprintf("%s/%s", provID, m.ID)),
		Name:    m.Name,
	}
	if m.ContextWindow > 0 && m.DefaultMaxTokens > 0 {
		ctxK := m.ContextWindow / 1024
		outK := m.DefaultMaxTokens / 1024
		if ctxK >= 1000 {
			info.Description = fmt.Sprintf("%dM ctx / %dK out", ctxK/1024, outK)
		} else {
			info.Description = fmt.Sprintf("%dK ctx / %dK out", ctxK, outK)
		}
	}
	return info
}
