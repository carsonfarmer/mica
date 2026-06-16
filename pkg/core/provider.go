package core

import (
	"fmt"
	"os"
	"strings"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"charm.land/fantasy/providers/anthropic"
	"charm.land/fantasy/providers/azure"
	"charm.land/fantasy/providers/bedrock"
	"charm.land/fantasy/providers/google"
	"charm.land/fantasy/providers/openai"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/fantasy/providers/openrouter"
	"charm.land/fantasy/providers/vercel"
)

func NewFantasyProvider(cw catwalk.Provider) (fantasy.Provider, error) {
	key := cw.APIKey
	if after, ok := strings.CutPrefix(key, "$"); ok {
		key = os.Getenv(after)
	}
	if key == "" {
		return nil, fmt.Errorf("no API key for provider %q (env %s)", cw.ID, cw.APIKey)
	}
	endpoint := cw.APIEndpoint
	if after, ok := strings.CutPrefix(endpoint, "$"); ok {
		endpoint = os.Getenv(after)
	}
	switch cw.Type {
	case catwalk.TypeOpenAI:
		return openai.New(openai.WithAPIKey(key), openai.WithBaseURL(endpoint))
	case catwalk.TypeOpenAICompat:
		return openaicompat.New(openaicompat.WithAPIKey(key), openaicompat.WithBaseURL(endpoint))
	case catwalk.TypeOpenRouter:
		return openrouter.New(openrouter.WithAPIKey(key))
	case catwalk.TypeVercel:
		return vercel.New(vercel.WithAPIKey(key), vercel.WithBaseURL(endpoint))
	case catwalk.TypeAnthropic:
		return anthropic.New(anthropic.WithAPIKey(key), anthropic.WithBaseURL(endpoint))
	case catwalk.TypeGoogle:
		return google.New(google.WithGeminiAPIKey(key), google.WithBaseURL(endpoint))
	case catwalk.TypeAzure:
		return azure.New(azure.WithAPIKey(key), azure.WithBaseURL(endpoint))
	case catwalk.TypeBedrock:
		return bedrock.New(bedrock.WithAPIKey(key), bedrock.WithBaseURL(endpoint))
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cw.Type)
	}
}
