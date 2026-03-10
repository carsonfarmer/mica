// Package store provides file-backed session persistence.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	acp "github.com/coder/acp-go-sdk"

	"github.com/carsonfarmer/mica/internal/app"
	"github.com/carsonfarmer/mica/pkg/session"
)

// FileStore implements session.Store using one JSONL file per session.
type FileStore struct{}

// NewFileStore constructs a file-backed session store.
func NewFileStore() *FileStore {
	return &FileStore{}
}

// Create writes the first header record for a session.
func (fs *FileStore) Create(header session.Header) error {
	path := app.SessionLogFile(header.Cwd, string(header.SessionId))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create sessions directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("create session log: %w", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(header); err != nil {
		return fmt.Errorf("encode session header: %w", err)
	}
	return nil
}

// Append writes an update record to an existing session log.
func (fs *FileStore) Append(cwd string, sessionID acp.SessionId, rec session.UpdateRecord) error {
	path := app.SessionLogFile(cwd, string(sessionID))
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open session log: %w", err)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(rec); err != nil {
		return fmt.Errorf("encode session update: %w", err)
	}
	return nil
}

// Load returns the session header and all persisted updates.
func (fs *FileStore) Load(cwd string, sessionID acp.SessionId) (session.Header, []session.UpdateRecord, error) {
	path := app.SessionLogFile(cwd, string(sessionID))
	file, err := os.Open(path)
	if err != nil {
		return session.Header{}, nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	var header session.Header
	if err := decoder.Decode(&header); err != nil {
		return session.Header{}, nil, fmt.Errorf("decode session header: %w", err)
	}

	var records []session.UpdateRecord
	for {
		var rec session.UpdateRecord
		if err := decoder.Decode(&rec); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return session.Header{}, nil, fmt.Errorf("decode session update: %w", err)
		}
		records = append(records, rec)
	}

	return header, records, nil
}

// List returns session IDs for cwd.
func (fs *FileStore) List(cwd string) ([]acp.SessionId, error) {
	dir := app.SessionsDir(cwd)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions directory: %w", err)
	}

	ids := make([]acp.SessionId, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != app.LogFileExt {
			continue
		}
		ids = append(ids, acp.SessionId(name[:len(name)-len(app.LogFileExt)]))
	}
	return ids, nil
}
