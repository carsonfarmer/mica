package session_test

import (
	"os"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/carsonfarmer/mica/internal/app"
	"github.com/carsonfarmer/mica/pkg/session"
)

type memoryStore struct {
	headers map[acp.SessionId]session.Header
	updates map[acp.SessionId][]session.UpdateRecord
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		headers: make(map[acp.SessionId]session.Header),
		updates: make(map[acp.SessionId][]session.UpdateRecord),
	}
}

func (s *memoryStore) Create(header session.Header) error {
	s.headers[header.SessionId] = header
	return nil
}

func (s *memoryStore) Append(cwd string, sessionID acp.SessionId, rec session.UpdateRecord) error {
	s.updates[sessionID] = append(s.updates[sessionID], rec)
	return nil
}

func (s *memoryStore) Load(cwd string, sessionID acp.SessionId) (session.Header, []session.UpdateRecord, error) {
	header, ok := s.headers[sessionID]
	if !ok {
		return session.Header{}, nil, os.ErrNotExist
	}
	return header, append([]session.UpdateRecord(nil), s.updates[sessionID]...), nil
}

func (s *memoryStore) List(cwd string) ([]acp.SessionId, error) {
	ids := make([]acp.SessionId, 0, len(s.headers))
	for id, header := range s.headers {
		if header.Cwd == cwd {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func TestLogsCreateAppendAndLoad(t *testing.T) {
	store := newMemoryStore()
	logs := session.NewLogs(store)

	log, err := logs.Create(testHeader("sess-1"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	title := "Hello"
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := log.Append(acp.SessionUpdate{
		SessionInfoUpdate: &acp.SessionSessionInfoUpdate{
			Title:     &title,
			UpdatedAt: &updatedAt,
		},
	}); err != nil {
		t.Fatalf("Append session info: %v", err)
	}
	if err := log.Append(acp.SessionUpdate{
		ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
			ConfigOptions: []acp.SessionConfigOption{selectOption("response_format", "raw")},
		},
	}); err != nil {
		t.Fatalf("Append config update: %v", err)
	}
	if err := log.Append(acp.SessionUpdate{
		CurrentModeUpdate: &acp.SessionCurrentModeUpdate{CurrentModeId: app.DefaultSessionModeID},
	}); err != nil {
		t.Fatalf("Append mode update: %v", err)
	}

	reloaded, err := session.NewLogs(store).Load("/tmp/project", "sess-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	info := reloaded.Info()
	if info.Title == nil || *info.Title != title {
		t.Fatalf("Title = %v, want %q", info.Title, title)
	}
	if info.UpdatedAt == nil || *info.UpdatedAt != updatedAt {
		t.Fatalf("UpdatedAt = %v, want %q", info.UpdatedAt, updatedAt)
	}
	options := reloaded.ConfigOptions()
	if len(options) != 1 || options[0].Select == nil || options[0].Select.CurrentValue != "raw" {
		t.Fatalf("ConfigOptions = %#v", options)
	}
	modes := reloaded.Modes()
	if modes == nil || modes.CurrentModeId != app.DefaultSessionModeID {
		t.Fatalf("Modes = %#v", modes)
	}
	if got := len(reloaded.Updates()); got != 3 {
		t.Fatalf("Updates length = %d, want 3", got)
	}
}

func TestLogsForkCopiesUpdates(t *testing.T) {
	store := newMemoryStore()
	logs := session.NewLogs(store)

	parent, err := logs.Create(testHeader("parent"))
	if err != nil {
		t.Fatalf("Create parent: %v", err)
	}
	if err := parent.Append(acp.UpdateUserMessageText("hello")); err != nil {
		t.Fatalf("Append parent update: %v", err)
	}

	parentID := acp.SessionId("parent")
	header := testHeader("child")
	header.SessionEvent = app.SessionEventFork
	header.ParentSessionId = &parentID
	child, err := logs.Fork("/tmp/project", parentID, header)
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}

	if got := len(child.Updates()); got != 1 {
		t.Fatalf("Child updates = %d, want 1", got)
	}
	childHeader := child.Header()
	if childHeader.ParentSessionId == nil || *childHeader.ParentSessionId != parentID {
		t.Fatalf("ParentSessionId = %v, want %q", childHeader.ParentSessionId, parentID)
	}
}

func TestLogsListReturnsDerivedInfo(t *testing.T) {
	store := newMemoryStore()
	logs := session.NewLogs(store)

	for _, id := range []acp.SessionId{"a", "b"} {
		log, err := logs.Create(testHeader(id))
		if err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
		title := "title-" + string(id)
		updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
		if err := log.Append(acp.SessionUpdate{
			SessionInfoUpdate: &acp.SessionSessionInfoUpdate{
				Title:     &title,
				UpdatedAt: &updatedAt,
			},
		}); err != nil {
			t.Fatalf("Append info %s: %v", id, err)
		}
	}

	infos, err := logs.List("/tmp/project")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("List length = %d, want 2", len(infos))
	}
	if infos[0].SessionId != "a" || infos[1].SessionId != "b" {
		t.Fatalf("Session order = %#v", infos)
	}
}

func TestLogsGetAndModels(t *testing.T) {
	store := newMemoryStore()
	logs := session.NewLogs(store)

	header := testHeader("sess-1")
	header.Models = &acp.UnstableSessionModelState{
		AvailableModels: []acp.UnstableModelInfo{{ModelId: acp.UnstableModelId(app.DefaultModelID), Name: app.DefaultModelName}},
		CurrentModelId:  acp.UnstableModelId(app.DefaultModelID),
	}
	created, err := logs.Create(header)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, ok := logs.Get("sess-1")
	if !ok || got != created {
		t.Fatalf("Get = %v, %v", got, ok)
	}
	models := created.Models()
	if models == nil || models.CurrentModelId != acp.UnstableModelId(app.DefaultModelID) {
		t.Fatalf("Models = %#v", models)
	}
}

func TestHeaderAndModelsReturnCopies(t *testing.T) {
	store := newMemoryStore()
	logs := session.NewLogs(store)

	header := testHeader("sess-1")
	header.Models = &acp.UnstableSessionModelState{
		AvailableModels: []acp.UnstableModelInfo{{ModelId: acp.UnstableModelId(app.DefaultModelID), Name: app.DefaultModelName}},
		CurrentModelId:  acp.UnstableModelId(app.DefaultModelID),
	}
	log, err := logs.Create(header)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotHeader := log.Header()
	gotHeader.ConfigOptions[0].Select.CurrentValue = "raw"
	gotHeader.Modes.CurrentModeId = "other"

	gotModels := log.Models()
	gotModels.CurrentModelId = "other-model"

	if log.Header().ConfigOptions[0].Select.CurrentValue != "echo" {
		t.Fatal("Header exposed mutable config options")
	}
	if log.Modes().CurrentModeId != app.DefaultSessionModeID {
		t.Fatal("Header exposed mutable modes")
	}
	if log.Models().CurrentModelId != acp.UnstableModelId(app.DefaultModelID) {
		t.Fatal("Models exposed mutable state")
	}
}

func TestLogsCreateCopiesInputHeader(t *testing.T) {
	store := newMemoryStore()
	logs := session.NewLogs(store)

	header := testHeader("sess-2")
	metaValue := "v1"
	header.Meta = map[string]any{"key": metaValue}
	created, err := logs.Create(header)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	header.ConfigOptions[0].Select.CurrentValue = "raw"
	header.Modes.CurrentModeId = "other"
	header.Meta["key"] = "v2"

	got := created.Header()
	if got.ConfigOptions[0].Select.CurrentValue != "echo" {
		t.Fatal("Create kept caller-owned config option state")
	}
	if got.Modes.CurrentModeId != app.DefaultSessionModeID {
		t.Fatal("Create kept caller-owned mode state")
	}
	if got.Meta["key"] != "v1" {
		t.Fatal("Create kept caller-owned meta map")
	}
}

func TestLogsCreateRejectsInvalidHeader(t *testing.T) {
	store := newMemoryStore()
	logs := session.NewLogs(store)

	if _, err := logs.Create(session.Header{SessionId: "sess-1", Cwd: "/tmp/project"}); err == nil {
		t.Fatal("Create accepted invalid header")
	}
}

func selectOption(id acp.SessionConfigId, current acp.SessionConfigValueId) acp.SessionConfigOption {
	options := acp.SessionConfigSelectOptionsUngrouped{{Name: string(current), Value: current}}
	return acp.SessionConfigOption{
		Select: &acp.SessionConfigOptionSelect{
			Id:           id,
			Name:         string(id),
			CurrentValue: current,
			Options: acp.SessionConfigSelectOptions{
				Ungrouped: &options,
			},
			Type: "select",
		},
	}
}

func testHeader(sessionID acp.SessionId) session.Header {
	return session.Header{
		EventID:       "evt-" + string(sessionID),
		Timestamp:     time.Date(2026, 3, 10, 6, 0, 0, 0, time.UTC),
		SessionEvent:  app.SessionEventNew,
		Cwd:           "/tmp/project",
		ConfigOptions: []acp.SessionConfigOption{selectOption("response_format", "echo")},
		Modes: &acp.SessionModeState{
			AvailableModes: []acp.SessionMode{{Id: app.DefaultSessionModeID, Name: app.DefaultSessionModeName}},
			CurrentModeId:  app.DefaultSessionModeID,
		},
		SessionId: sessionID,
	}
}
