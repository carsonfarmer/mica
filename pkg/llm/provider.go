package llm

import (
	"fmt"
	"os"
	"strings"

	"charm.land/catwalk/pkg/catwalk"
	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
)

func newFantasyProvider(cw catwalk.Provider) (fantasy.Provider, error) {
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
	case catwalk.TypeOpenAI, catwalk.TypeOpenAICompat, catwalk.TypeOpenRouter, catwalk.TypeVercel:
		return openaicompat.New(
			openaicompat.WithAPIKey(key),
			openaicompat.WithBaseURL(endpoint),
		)
	default:
		return nil, fmt.Errorf("unsupported provider type %q", cw.Type)
	}
}
