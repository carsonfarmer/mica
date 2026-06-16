package agent

import (
	"context"
	"testing"

	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/mica/pkg/core"
)

func TestGetSessionConfigOptions(t *testing.T) {
	reg := newTestRegistry(t)
	store := storage.NewTypedMemoryStore[*core.AgentSession]()
	a := New(reg, store)

	// Session with the fast model (no reasoning).
	sess := &core.AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: "s1", CWD: "/tmp"},
		Model:       core.FullModelID("testai/test-model-fast"),
	}
	opts := a.getSessionConfigOptions(sess)
	if len(opts) == 0 {
		t.Fatal("expected config options for fast model")
	}
	// Should have model selector, no reasoning selector.
	hasModel := false
	hasReasoning := false
	for _, o := range opts {
		if o.Select == nil {
			continue
		}
		switch o.Select.Category {
		case acp.ConfigCatModel:
			hasModel = true
		case acp.ConfigCatThoughtLevel:
			hasReasoning = true
		}
	}
	if !hasModel {
		t.Fatal("expected model selector")
	}
	if hasReasoning {
		t.Fatal("did not expect reasoning selector for non-reasoning model")
	}

	// Session with reasoning model.
	sess.Model = core.FullModelID("testai/test-model-reason")
	opts = a.getSessionConfigOptions(sess)
	hasReasoning = false
	for _, o := range opts {
		if o.Select != nil && o.Select.Category == acp.ConfigCatThoughtLevel {
			hasReasoning = true
		}
	}
	if !hasReasoning {
		t.Fatal("expected reasoning selector for reasoning model")
	}
}

func TestSetModelConfigOption(t *testing.T) {
	reg := newTestRegistry(t)
	store := storage.NewTypedMemoryStore[*core.AgentSession]()
	sess := &core.AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: "s1", CWD: "/tmp"},
		Model:       core.FullModelID("testai/test-model-fast"),
	}
	if err := store.Set(context.Background(), "s1", sess); err != nil {
		t.Fatal(err)
	}
	a := New(reg, store)

	resp, err := a.SetSessionConfigOption(context.Background(), &acp.SetSessionConfigOptionRequest{
		SessionID: "s1",
		ConfigID:  string(acp.ConfigCatModel),
		Value:     "testai/test-model-reason",
	}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}

	// Invalid model
	_, err = a.SetSessionConfigOption(context.Background(), &acp.SetSessionConfigOptionRequest{
		SessionID: "s1",
		ConfigID:  string(acp.ConfigCatModel),
		Value:     "noprovider/model",
	}, nopClient{})
	if err == nil {
		t.Fatal("expected error for invalid model")
	}

	// Invalid config ID
	_, err = a.SetSessionConfigOption(context.Background(), &acp.SetSessionConfigOptionRequest{
		SessionID: "s1",
		ConfigID:  "unknown",
		Value:     "x",
	}, nopClient{})
	if err == nil {
		t.Fatal("expected error for unknown config")
	}
}

func TestSetThoughtLevelConfigOption(t *testing.T) {
	reg := newTestRegistry(t)
	store := storage.NewTypedMemoryStore[*core.AgentSession]()
	sess := &core.AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: "s1", CWD: "/tmp"},
		Model:       core.FullModelID("testai/test-model-reason"),
	}
	if err := store.Set(context.Background(), "s1", sess); err != nil {
		t.Fatal(err)
	}
	a := New(reg, store)

	resp, err := a.SetSessionConfigOption(context.Background(), &acp.SetSessionConfigOptionRequest{
		SessionID: "s1",
		ConfigID:  string(acp.ConfigCatThoughtLevel),
		Value:     "high",
	}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}

	// Invalid level
	_, err = a.SetSessionConfigOption(context.Background(), &acp.SetSessionConfigOptionRequest{
		SessionID: "s1",
		ConfigID:  string(acp.ConfigCatThoughtLevel),
		Value:     "extreme",
	}, nopClient{})
	if err == nil {
		t.Fatal("expected error for invalid reasoning level")
	}

	// Non-reasoning model — setting thought level should fail
	sess2 := &core.AgentSession{
		SessionInfo: &acp.SessionInfo{SessionID: "s2", CWD: "/tmp"},
		Model:       core.FullModelID("testai/test-model-fast"),
	}
	if err := store.Set(context.Background(), "s2", sess2); err != nil {
		t.Fatal(err)
	}
	_, err = a.SetSessionConfigOption(context.Background(), &acp.SetSessionConfigOptionRequest{
		SessionID: "s2",
		ConfigID:  string(acp.ConfigCatThoughtLevel),
		Value:     "low",
	}, nopClient{})
	if err == nil {
		t.Fatal("expected error for reasoning level on non-reasoning model")
	}
}
