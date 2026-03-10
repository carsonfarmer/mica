package store

import (
	"os"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"

	"github.com/carsonfarmer/mica/internal/app"
	"github.com/carsonfarmer/mica/pkg/session"
)

func TestFileStoreCreateLoadAndList(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	header := session.Header{
		EventID:      "evt-header",
		Timestamp:    time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC),
		SessionEvent: app.SessionEventNew,
		Cwd:          tmpDir,
		SessionId:    "sess-1",
	}
	if err := store.Create(header); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(tmpDir, "sess-1", session.UpdateRecord{
		EventID:   "evt-1",
		Timestamp: time.Date(2026, 3, 9, 10, 1, 0, 0, time.UTC),
		Update:    acp.UpdateUserMessageText("hello"),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	loadedHeader, updates, err := store.Load(tmpDir, "sess-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loadedHeader.EventID != "evt-header" {
		t.Fatalf("Header EventID = %q", loadedHeader.EventID)
	}
	if len(updates) != 1 || updates[0].EventID != "evt-1" {
		t.Fatalf("Updates = %#v", updates)
	}

	ids, err := store.List(tmpDir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 1 || ids[0] != "sess-1" {
		t.Fatalf("IDs = %#v", ids)
	}
}

func TestFileStoreLoadIgnoresTrailingTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	if err := store.Create(session.Header{
		EventID:      "evt-header-2",
		Timestamp:    time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC),
		SessionEvent: app.SessionEventNew,
		Cwd:          tmpDir,
		SessionId:    "sess-1",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(tmpDir, "sess-1", session.UpdateRecord{
		EventID:   "evt-1",
		Timestamp: time.Now().UTC(),
		Update:    acp.UpdateAgentMessageText("hello"),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	path := app.SessionLogFile(tmpDir, "sess-1")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := file.WriteString("{\"eventId\":\"partial\""); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, updates, err := store.Load(tmpDir, "sess-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("Updates length = %d, want 1", len(updates))
	}
}

func TestFileStoreLoadReturnsSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	if err := store.Create(session.Header{
		EventID:      "evt-header-4",
		Timestamp:    time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC),
		SessionEvent: app.SessionEventNew,
		Cwd:          tmpDir,
		SessionId:    "sess-1",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Append(tmpDir, "sess-1", session.UpdateRecord{
		EventID:   "evt-1",
		Timestamp: time.Now().UTC(),
		Update:    acp.UpdateAgentMessageText("hello"),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	path := app.SessionLogFile(tmpDir, "sess-1")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := file.WriteString("{\"eventId\": bad}\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, _, err := store.Load(tmpDir, "sess-1"); err == nil {
		t.Fatal("Load returned nil error for invalid JSON object")
	}
}

func TestFileStoreLoadMissing(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	if _, _, err := store.Load(tmpDir, "missing"); err == nil {
		t.Fatal("Load missing session returned nil error")
	}
}

func TestFileStoreListEmptyAndDuplicateCreate(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	ids, err := store.List(tmpDir)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("List empty length = %d, want 0", len(ids))
	}

	header := session.Header{
		EventID:      "evt-header-3",
		Timestamp:    time.Date(2026, 3, 9, 10, 0, 0, 0, time.UTC),
		SessionEvent: app.SessionEventNew,
		Cwd:          tmpDir,
		SessionId:    "sess-1",
	}
	if err := store.Create(header); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Create(header); err == nil {
		t.Fatal("duplicate Create returned nil error")
	}
}

func TestFileStoreAppendMissing(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileStore()

	if err := store.Append(tmpDir, "missing", session.UpdateRecord{
		EventID:   "evt-1",
		Timestamp: time.Now().UTC(),
		Update:    acp.UpdateUserMessageText("hello"),
	}); err == nil {
		t.Fatal("Append missing session returned nil error")
	}
}
