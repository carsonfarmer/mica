package core

import (
	"context"
	"fmt"
	"strings"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/google"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"
	acp "github.com/carsonfarmer/go-acp-sdk"
)

// FullModelID is a typed "provider/model" identifier.
type FullModelID string

type entry struct {
	catwalk catwalk.Provider
	fantasy fantasy.Provider
	models  map[string]catwalk.Model
}

// Registry maps catwalk providers to fantasy providers and their models.
type Registry struct {
	providers    map[catwalk.InferenceProvider]*entry
	defaultModel FullModelID
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[catwalk.InferenceProvider]*entry)}
}

// AddProvider creates a fantasy provider from the catwalk config and registers all models.
func (r *Registry) AddProvider(cw catwalk.Provider) error {
	prov, err := NewFantasyProvider(cw)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}
	return r.addEntry(cw, prov)
}

func (r *Registry) addEntry(cw catwalk.Provider, prov fantasy.Provider) error {
	ent := &entry{catwalk: cw, fantasy: prov, models: make(map[string]catwalk.Model)}
	for _, m := range cw.Models {
		ent.models[m.ID] = m
	}
	r.providers[cw.ID] = ent
	r.defaultModel = FullModelID(string(cw.ID) + "/" + cw.DefaultLargeModelID)
	return nil
}

// Resolve returns the LanguageModel for "provider/model".
func (r *Registry) Resolve(ctx context.Context, mid FullModelID) (fantasy.LanguageModel, error) {
	prov, model, _ := strings.Cut(string(mid), "/")
	ent, ok := r.providers[catwalk.InferenceProvider(prov)]
	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q", prov)
	}
	if ent.fantasy == nil {
		return nil, fmt.Errorf("llm: provider %q not available (check API key)", prov)
	}
	return ent.fantasy.LanguageModel(ctx, model)
}

// Config returns the catwalk model config for "provider/model".
func (r *Registry) Config(mid FullModelID) (catwalk.Model, bool) {
	prov, model, _ := strings.Cut(string(mid), "/")
	ent, ok := r.providers[catwalk.InferenceProvider(prov)]
	if !ok {
		return catwalk.Model{}, ok
	}
	m, ok := ent.models[model]
	return m, ok
}

// Default returns the default model ID.
func (r *Registry) Default() FullModelID { return r.defaultModel }

// Providers returns the list of registered providers as ACP ProviderInfo.
func (r *Registry) Providers() []acp.ProviderInfo {
	var out []acp.ProviderInfo
	for _, ent := range r.providers {
		apiType := TypeToACP(ent.catwalk.Type)
		out = append(out, acp.ProviderInfo{
			ID:        string(ent.catwalk.ID),
			Supported: []acp.LlmProtocol{apiType},
			Current: &acp.ProviderCurrentConfig{
				APIType: apiType,
				BaseURL: ent.catwalk.APIEndpoint,
			},
		})
	}
	return out
}

// ProviderOptions returns the fantasy provider options for a model and thought level.
func (r *Registry) ProviderOptions(mid FullModelID, thoughtLevel string) fantasy.ProviderOptions {
	if thoughtLevel == "" {
		return nil
	}
	prov, _, _ := strings.Cut(string(mid), "/")
	ent, ok := r.providers[catwalk.InferenceProvider(prov)]
	if !ok {
		return nil
	}

	switch ent.catwalk.Type {
	case catwalk.TypeOpenAI:
		effort := openai.ReasoningEffort(thoughtLevel)
		return fantasy.ProviderOptions{
			openai.Name: &openai.ProviderOptions{ReasoningEffort: &effort},
		}
	case catwalk.TypeOpenAICompat:
		effort := openai.ReasoningEffort(thoughtLevel)
		return fantasy.ProviderOptions{
			openaicompat.Name: &openaicompat.ProviderOptions{
				ReasoningEffort: &effort,
				ExtraBody: map[string]any{
					"interleaved": map[string]string{"field": "reasoning_content"},
				},
			},
		}
	case catwalk.TypeAnthropic:
		effort := anthropic.Effort(thoughtLevel)
		return fantasy.ProviderOptions{
			anthropic.Name: &anthropic.ProviderOptions{
				Effort:        &effort,
				SendReasoning: new(true),
			},
		}
	case catwalk.TypeGoogle:
		level := strings.ToUpper(thoughtLevel)
		return fantasy.ProviderOptions{
			google.Name: &google.ProviderOptions{
				ThinkingConfig: &google.ThinkingConfig{
					ThinkingLevel:   &level,
					IncludeThoughts: new(true),
				},
			},
		}
	case catwalk.TypeOpenRouter:
		effort := openrouter.ReasoningEffort(thoughtLevel)
		return fantasy.ProviderOptions{
			openrouter.Name: &openrouter.ProviderOptions{
				Reasoning: &openrouter.ReasoningOptions{
					Enabled: new(true),
					Effort:  &effort,
				},
			},
		}
	default:
		return nil
	}
}

// Models returns all models for a given provider as ACP ModelInfo.
func (r *Registry) Models(providerID string) []acp.ModelInfo {
	ent, ok := r.providers[catwalk.InferenceProvider(providerID)]
	if !ok {
		return nil
	}
	out := make([]acp.ModelInfo, 0, len(ent.catwalk.Models))
	for _, m := range ent.catwalk.Models {
		info := acp.ModelInfo{
			ModelID: acp.ModelID(fmt.Sprintf("%s/%s", ent.catwalk.ID, m.ID)),
			Name:    m.Name,
		}
		if m.ContextWindow > 0 {
			info.Description = fmt.Sprintf("%.1gM ctx", float64(m.ContextWindow)/1e6)
		}
		out = append(out, info)
	}
	return out
}
