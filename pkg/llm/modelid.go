package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/catwalk/pkg/catwalk"
)

// FullModelID is a typed "provider/model" identifier.
type FullModelID struct {
	Provider catwalk.InferenceProvider
	Model    string
}

// ParseFullModelID parses a "provider/model" string.
func ParseFullModelID(s string) (FullModelID, error) {
	prov, model, ok := strings.Cut(s, "/")
	if !ok {
		return FullModelID{}, fmt.Errorf("invalid model ID %q (want provider/model)", s)
	}
	return FullModelID{Provider: catwalk.InferenceProvider(prov), Model: model}, nil
}

// String returns "provider/model".
func (m FullModelID) String() string {
	return fmt.Sprintf("%s/%s", m.Provider, m.Model)
}

// MarshalJSON renders as "provider/model".
func (m FullModelID) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// UnmarshalJSON parses "provider/model" from JSON.
func (m *FullModelID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := ParseFullModelID(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
