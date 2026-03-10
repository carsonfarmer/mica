package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/carsonfarmer/mica/internal/app"
	"github.com/carsonfarmer/mica/pkg/session"
	"github.com/carsonfarmer/mica/pkg/session/store"
)

func TestInitialize(t *testing.T) {
	ag := newTestAgent(t)

	resp, err := ag.Initialize(context.Background(), acp.InitializeRequest{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if resp.AgentInfo == nil || resp.AgentInfo.Name != app.AgentName {
		t.Fatalf("AgentInfo = %#v", resp.AgentInfo)
	}
	if !resp.AgentCapabilities.LoadSession {
		t.Fatal("LoadSession capability is false")
	}
	if resp.AgentCapabilities.SessionCapabilities.Fork == nil {
		t.Fatal("Fork capability is nil")
	}
}

func TestAuthenticateAndSetAgentConnection(t *testing.T) {
	ag := newTestAgent(t)
	ag.SetAgentConnection(nil)

	if _, err := ag.Authenticate(context.Background(), acp.AuthenticateRequest{}); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
}

func TestNewSessionAndLoadSession(t *testing.T) {
	ag := newTestAgent(t)
	tmpDir := t.TempDir()

	newResp, err := ag.NewSession(context.Background(), acp.NewSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if newResp.SessionId == "" {
		t.Fatal("SessionId is empty")
	}
	if len(newResp.ConfigOptions) != 3 {
		t.Fatalf("ConfigOptions length = %d, want 3", len(newResp.ConfigOptions))
	}
	if newResp.Models == nil || newResp.Models.CurrentModelId != app.DefaultModelID {
		t.Fatalf("Models = %#v", newResp.Models)
	}

	loadResp, err := ag.LoadSession(context.Background(), acp.LoadSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
		SessionId:  newResp.SessionId,
	})
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loadResp.Models == nil || loadResp.Models.CurrentModelId != app.DefaultModelID {
		t.Fatalf("Load Models = %#v", loadResp.Models)
	}
}

func TestLoadResumeAndConfigErrors(t *testing.T) {
	ag := newTestAgent(t)
	tmpDir := t.TempDir()

	if _, err := ag.LoadSession(context.Background(), acp.LoadSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
		SessionId:  "missing",
	}); err == nil || !strings.Contains(err.Error(), "Invalid params") {
		t.Fatalf("LoadSession missing error = %v", err)
	}
	if _, err := ag.UnstableResumeSession(context.Background(), acp.UnstableResumeSessionRequest{
		Cwd:       tmpDir,
		SessionId: "missing",
	}); err == nil || !strings.Contains(err.Error(), "Invalid params") {
		t.Fatalf("UnstableResumeSession missing error = %v", err)
	}

	newResp, err := ag.NewSession(context.Background(), acp.NewSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if _, err := ag.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		SessionId: newResp.SessionId,
		ConfigId:  "unknown",
		Value:     "value",
	}); err == nil || !strings.Contains(err.Error(), "Invalid params") {
		t.Fatalf("unknown config error = %v", err)
	}
	if _, err := ag.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		SessionId: newResp.SessionId,
		ConfigId:  responseFormatConfigID,
		Value:     "bad",
	}); err == nil || !strings.Contains(err.Error(), "Invalid params") {
		t.Fatalf("bad value error = %v", err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previous)
	})

	listResp, err := ag.UnstableListSessions(context.Background(), acp.UnstableListSessionsRequest{})
	if err != nil {
		t.Fatalf("UnstableListSessions default cwd: %v", err)
	}
	if len(listResp.Sessions) != 1 {
		t.Fatalf("Sessions length = %d, want 1", len(listResp.Sessions))
	}
}

func TestPromptPersistsSessionUpdates(t *testing.T) {
	ag := newTestAgent(t)
	tmpDir := t.TempDir()

	newResp, err := ag.NewSession(context.Background(), acp.NewSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	resp, err := ag.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hello")},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if resp.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("StopReason = %q", resp.StopReason)
	}

	log, err := ag.logs.Load(tmpDir, newResp.SessionId)
	if err != nil {
		t.Fatalf("Load log: %v", err)
	}
	updates := log.Updates()
	if len(updates) != 3 {
		t.Fatalf("Updates length = %d, want 3", len(updates))
	}
	if updates[1].UserMessageChunk == nil || textFromBlock(updates[1].UserMessageChunk.Content) != "hello" {
		t.Fatalf("User update = %#v", updates[1])
	}
	if updates[2].AgentMessageChunk == nil || textFromBlock(updates[2].AgentMessageChunk.Content) != "Echo: hello" {
		t.Fatalf("Agent update = %#v", updates[2])
	}
}

func TestSetSessionConfigOptionUpdatesRawAndMode(t *testing.T) {
	ag := newTestAgent(t)
	tmpDir := t.TempDir()

	newResp, err := ag.NewSession(context.Background(), acp.NewSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	setResp, err := ag.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		SessionId: newResp.SessionId,
		ConfigId:  responseFormatConfigID,
		Value:     rawResponseFormat,
	})
	if err != nil {
		t.Fatalf("SetSessionConfigOption raw: %v", err)
	}
	if currentValue(setResp.ConfigOptions, responseFormatConfigID) != rawResponseFormat {
		t.Fatalf("ConfigOptions = %#v", setResp.ConfigOptions)
	}

	if _, err := ag.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		SessionId: newResp.SessionId,
		ConfigId:  modeConfigID,
		Value:     acp.SessionConfigValueId(defaultModeID),
	}); err != nil {
		t.Fatalf("SetSessionConfigOption mode: %v", err)
	}

	promptResp, err := ag.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("raw text")},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if promptResp.StopReason != acp.StopReasonEndTurn {
		t.Fatalf("Prompt stop reason = %q", promptResp.StopReason)
	}

	log, err := ag.logs.Load(tmpDir, newResp.SessionId)
	if err != nil {
		t.Fatalf("Load log: %v", err)
	}
	updates := log.Updates()
	foundModeUpdate := false
	foundRawAgent := false
	for _, update := range updates {
		if update.CurrentModeUpdate != nil && update.CurrentModeUpdate.CurrentModeId == defaultModeID {
			foundModeUpdate = true
		}
		if update.AgentMessageChunk != nil && textFromBlock(update.AgentMessageChunk.Content) == "raw text" {
			foundRawAgent = true
		}
	}
	if !foundModeUpdate {
		t.Fatal("expected current_mode_update in persisted updates")
	}
	if !foundRawAgent {
		t.Fatal("expected raw agent response in persisted updates")
	}
}

func TestUnstableForkResumeAndList(t *testing.T) {
	ag := newTestAgent(t)
	tmpDir := t.TempDir()

	newResp, err := ag.NewSession(context.Background(), acp.NewSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err := ag.Prompt(context.Background(), acp.PromptRequest{
		SessionId: newResp.SessionId,
		Prompt:    []acp.ContentBlock{acp.TextBlock("fork me")},
	}); err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	forkResp, err := ag.UnstableForkSession(context.Background(), acp.UnstableForkSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
		SessionId:  newResp.SessionId,
	})
	if err != nil {
		t.Fatalf("UnstableForkSession: %v", err)
	}
	if forkResp.SessionId == "" || forkResp.SessionId == newResp.SessionId {
		t.Fatalf("Fork session id = %q", forkResp.SessionId)
	}

	resumeResp, err := ag.UnstableResumeSession(context.Background(), acp.UnstableResumeSessionRequest{
		Cwd:        tmpDir,
		McpServers: []acp.McpServer{},
		SessionId:  forkResp.SessionId,
	})
	if err != nil {
		t.Fatalf("UnstableResumeSession: %v", err)
	}
	if resumeResp.Models == nil || resumeResp.Models.CurrentModelId != acp.UnstableModelId(app.DefaultModelID) {
		t.Fatalf("Resume models = %#v", resumeResp.Models)
	}

	listResp, err := ag.UnstableListSessions(context.Background(), acp.UnstableListSessionsRequest{
		Cwd: &tmpDir,
	})
	if err != nil {
		t.Fatalf("UnstableListSessions: %v", err)
	}
	if len(listResp.Sessions) != 2 {
		t.Fatalf("Sessions length = %d, want 2", len(listResp.Sessions))
	}
}

func TestUnsupportedMethods(t *testing.T) {
	ag := newTestAgent(t)

	if _, err := ag.SetSessionMode(context.Background(), acp.SetSessionModeRequest{}); err == nil || !strings.Contains(err.Error(), "Method not found") {
		t.Fatalf("SetSessionMode error = %v", err)
	}
	if _, err := ag.UnstableSetSessionModel(context.Background(), acp.UnstableSetSessionModelRequest{}); err == nil || !strings.Contains(err.Error(), "Method not found") {
		t.Fatalf("UnstableSetSessionModel error = %v", err)
	}
}

func TestPromptRequiresLoadedSession(t *testing.T) {
	ag := newTestAgent(t)

	if _, err := ag.Prompt(context.Background(), acp.PromptRequest{
		SessionId: "missing",
		Prompt:    []acp.ContentBlock{acp.TextBlock("hello")},
	}); err == nil || !strings.Contains(err.Error(), "Invalid params") {
		t.Fatalf("Prompt error = %v", err)
	}
}

func TestFlattenPromptAndChunking(t *testing.T) {
	flattened := flattenPrompt([]acp.ContentBlock{
		acp.TextBlock("hello"),
		acp.ResourceLinkBlock("spec", "file:///tmp/spec.md"),
	})
	if flattened != "hello\nspec file:///tmp/spec.md" {
		t.Fatalf("flattenPrompt = %q", flattened)
	}

	chunks := splitIntoChunks("你好世界", 2)
	if len(chunks) != 2 || chunks[0] != "你好" || chunks[1] != "世界" {
		t.Fatalf("splitIntoChunks = %#v", chunks)
	}
}

func TestCancel(t *testing.T) {
	ag := newTestAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ag.setCancel("sess", cancel)
	if err := ag.Cancel(context.Background(), acp.CancelNotification{SessionId: "sess"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cancel did not propagate")
	}
}

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	fileStore := store.NewFileStore()
	logs := session.NewLogs(fileStore)
	return New(logs)
}

func currentValue(options []acp.SessionConfigOption, id acp.SessionConfigId) acp.SessionConfigValueId {
	for _, option := range options {
		if option.Select != nil && option.Select.Id == id {
			return option.Select.CurrentValue
		}
	}
	return ""
}

func textFromBlock(block acp.ContentBlock) string {
	if block.Text != nil {
		return block.Text.Text
	}
	return ""
}
