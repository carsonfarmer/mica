// Package llm bridges the ACP protocol (go-acp-sdk) with LLM providers
// (charm.land/fantasy). Callers never see Fantasy types — the package wraps
// them in ACP-native APIs.
package llm

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
)

// Registry tracks available providers and their models. It is programmatic:
// callers register providers and models in code. There is no JSON config
// parsing at this layer.
type Registry struct {
	providers    map[string]entry
	defaultModel string
}

// ModelGroup is one provider with its selectable models.
type ModelGroup struct {
	Info   acp.ProviderInfo
	Models []acp.ModelInfo
}

type entry struct {
	ModelGroup
	provider fantasy.Provider
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]entry)}
}

// Register adds a provider and its selectable models.
// info supplies the ACP ProviderInfo (ID, Supported, Required, etc.).
// Each model's ModelID must be in "provider/model" form.
// The last model registered becomes the default.
func (r *Registry) Register(info acp.ProviderInfo, provider fantasy.Provider, models ...acp.ModelInfo) {
	r.providers[info.ID] = entry{
		ModelGroup: ModelGroup{Info: info, Models: models},
		provider:   provider,
	}
	if len(models) > 0 {
		r.defaultModel = string(models[len(models)-1].ModelID)
	}
}

// SetDefault overrides the default model ID ("provider/model").
func (r *Registry) SetDefault(modelID string) {
	r.defaultModel = modelID
}

// Default returns the default model ID, or "".
func (r *Registry) Default() string {
	return r.defaultModel
}

// Resolve looks up "provider/model" and returns a LanguageModel ready to call.
func (r *Registry) Resolve(ctx context.Context, modelID string) (fantasy.LanguageModel, error) {
	provName, modelName, ok := strings.Cut(modelID, "/")
	if !ok {
		return nil, fmt.Errorf("llm: invalid model ID %q (want provider/model)", modelID)
	}
	ent, ok := r.providers[provName]
	if !ok {
		return nil, fmt.Errorf("llm: unknown provider %q", provName)
	}
	return ent.provider.LanguageModel(ctx, modelName)
}

// Available returns models grouped by provider for building
// ACP session/set_model options.
func (r *Registry) Available() []ModelGroup {
	var groups []ModelGroup
	for _, ent := range r.providers {
		groups = append(groups, ent.ModelGroup)
	}
	return groups
}

// ProviderInfos returns the list of registered providers as ACP ProviderInfo values.
func (r *Registry) ProviderInfos() []acp.ProviderInfo {
	var infos []acp.ProviderInfo
	for _, ent := range r.providers {
		infos = append(infos, ent.Info)
	}
	return infos
}
