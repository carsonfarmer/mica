package agent

import (
	"context"
	"testing"

	"charm.land/catwalk/pkg/catwalk"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/mica/pkg/core"
)

func newTestRegistry(t *testing.T) *core.Registry {
	t.Setenv("TESTAI_API_KEY", "test-key")
	reg := core.NewRegistry()
	if err := reg.AddProvider(catwalk.Provider{
		ID:                  "testai",
		Name:                "TestAI",
		Type:                catwalk.TypeOpenAI,
		APIKey:              "$TESTAI_API_KEY",
		APIEndpoint:         "https://api.testai.example/v1",
		DefaultLargeModelID: "test-model-fast",
		Models: []catwalk.Model{
			{
				ID:            "test-model-fast",
				Name:          "Test Model Fast",
				ContextWindow: 128_000,
				CostPer1MIn:  3.00,
				CostPer1MOut:  15.00,
			},
			{
				ID:                     "test-model-reason",
				Name:                   "Test Model Reason",
				ContextWindow:          256_000,
				CostPer1MIn:            5.00,
				CostPer1MOut:           20.00,
				CanReason:              true,
				ReasoningLevels:        []string{"low", "medium", "high"},
				DefaultReasoningEffort: "medium",
			},
		},
	}); err != nil {
		t.Fatalf("AddProvider: %v", err)
	}
	return reg
}

func newTestAgent(t *testing.T) *Agent {
	store := storage.NewTypedMemoryStore[*core.AgentSession]()
	return New(newTestRegistry(t), store, WithName("mica-test"))
}

// nopClient implements acp.Client with no-ops for all methods.
type nopClient struct{}

func (nopClient) SessionUpdate(context.Context, *acp.SessionNotification) error     { return nil }
func (nopClient) RequestPermission(_ context.Context, req *acp.RequestPermissionRequest) (*acp.RequestPermissionResponse, error) {
	return &acp.RequestPermissionResponse{Outcome: acp.PermissionCancelled()}, nil
}
func (nopClient) ReadTextFile(context.Context, *acp.ReadTextFileRequest) (*acp.ReadTextFileResponse, error) {
	return &acp.ReadTextFileResponse{}, nil
}
func (nopClient) WriteTextFile(context.Context, *acp.WriteTextFileRequest) (*acp.WriteTextFileResponse, error) {
	return &acp.WriteTextFileResponse{}, nil
}

var _ acp.Client = nopClient{}

func TestInitialize(t *testing.T) {
	a := newTestAgent(t)
	resp, err := a.Initialize(context.Background(), &acp.InitializeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ProtocolVersion != 1 {
		t.Fatalf("ProtocolVersion = %d, want 1", resp.ProtocolVersion)
	}
	if resp.AgentInfo.Name != "mica-test" {
		t.Fatalf("Name = %q", resp.AgentInfo.Name)
	}
	if !resp.AgentCapabilities.LoadSession {
		t.Fatal("expected LoadSession capability")
	}
	if resp.AgentCapabilities.PromptCapabilities.EmbeddedContext != true {
		t.Fatal("expected EmbeddedContext capability")
	}
}

func TestAuthenticate(t *testing.T) {
	a := newTestAgent(t)
	resp, err := a.Authenticate(context.Background(), &acp.AuthenticateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestNewSession(t *testing.T) {
	a := newTestAgent(t)
	resp, err := a.NewSession(context.Background(), &acp.NewSessionRequest{
		CWD: "/tmp/test",
	}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.SessionID == "" {
		t.Fatal("expected SessionID")
	}
	if len(resp.ConfigOptions) == 0 {
		t.Fatal("expected config options")
	}
}

func TestLoadSession(t *testing.T) {
	a := newTestAgent(t)
	ctx := context.Background()

	newResp, err := a.NewSession(ctx, &acp.NewSessionRequest{}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := a.LoadSession(ctx, &acp.LoadSessionRequest{
		SessionID: newResp.SessionID,
	}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ConfigOptions) == 0 {
		t.Fatal("expected config options")
	}
}

func TestResumeSession(t *testing.T) {
	a := newTestAgent(t)
	ctx := context.Background()

	newResp, err := a.NewSession(ctx, &acp.NewSessionRequest{}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := a.ResumeSession(ctx, &acp.ResumeSessionRequest{
		SessionID: newResp.SessionID,
	}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.ConfigOptions) == 0 {
		t.Fatal("expected config options")
	}
}

func TestForkSession(t *testing.T) {
	a := newTestAgent(t)
	ctx := context.Background()

	newResp, err := a.NewSession(ctx, &acp.NewSessionRequest{CWD: "/fork"}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}

	forkResp, err := a.ForkSession(ctx, &acp.ForkSessionRequest{
		SessionID: newResp.SessionID,
	}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}
	if forkResp.SessionID == newResp.SessionID {
		t.Fatal("expected different session ID after fork")
	}
}

func TestCloseSession(t *testing.T) {
	a := newTestAgent(t)
	ctx := context.Background()

	newResp, err := a.NewSession(ctx, &acp.NewSessionRequest{}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.CloseSession(ctx, &acp.CloseSessionRequest{
		SessionID: newResp.SessionID,
	}, nopClient{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCancel(t *testing.T) {
	a := newTestAgent(t)
	err := a.Cancel(context.Background(), &acp.CancelNotification{SessionID: "nonexistent"})
	// Cancel is always a no-op for missing sessions — no error.
	if err != nil {
		t.Fatal(err)
	}
}

func TestListSessions(t *testing.T) {
	a := newTestAgent(t)
	ctx := context.Background()

	_, _ = a.NewSession(ctx, &acp.NewSessionRequest{}, nopClient{})
	_, _ = a.NewSession(ctx, &acp.NewSessionRequest{}, nopClient{})

	resp, err := a.ListSessions(ctx, &acp.ListSessionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
	}
}
