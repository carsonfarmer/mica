// Package session owns durable session state and replay over a storage backend.
package session

import (
	"errors"
	"maps"
	"slices"
	"sync"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"
)

// Header is the first durable record in a session log.
type Header struct {
	// The _meta property is reserved by ACP to allow clients and agents to attach additional
	// metadata to their interactions. Implementations MUST NOT make assumptions about values at
	// these keys.
	//
	// See protocol docs: [Extensibility](https://agentclientprotocol.com/protocol/extensibility)
	Meta map[string]any `json:"_meta,omitempty"`
	// Per-record UUID for durable identity.
	EventID string `json:"eventId"`
	// Timestamp when the header record was created.
	Timestamp time.Time `json:"ts"`
	// Session-origin discriminator such as "new" or "fork".
	SessionEvent string `json:"sessionEvent"`
	// The working directory for this session.
	Cwd string `json:"cwd"`
	// List of MCP servers to connect to for this session.
	McpServers []acp.McpServer `json:"mcpServers,omitempty"`
	// Initial session configuration options if supported by the Agent.
	ConfigOptions []acp.SessionConfigOption `json:"configOptions,omitempty"`
	// **UNSTABLE**
	//
	// This capability is not part of the spec yet, and may be removed or changed at any point.
	//
	// Initial model state if supported by the Agent
	Models *acp.UnstableSessionModelState `json:"models,omitempty"`
	// Initial mode state if supported by the Agent
	//
	// See protocol docs: [Session Modes](https://agentclientprotocol.com/protocol/session-modes)
	Modes *acp.SessionModeState `json:"modes,omitempty"`
	// Unique identifier for the created session.
	//
	// Used in all subsequent requests for this conversation.
	SessionId acp.SessionId `json:"sessionId"`
	// The parent session ID for forked sessions.
	ParentSessionId *acp.SessionId `json:"parentSessionId,omitempty"`
}

// UpdateRecord is a durable session update line after the header.
type UpdateRecord struct {
	// Per-record UUID for durable identity.
	EventID string `json:"eventId"`
	// Timestamp when the update record was created.
	Timestamp time.Time `json:"ts"`
	// The actual update content.
	Update acp.SessionUpdate `json:"update"`
}

// Store persists session headers and update records.
type Store interface {
	// Create writes the first durable header record for a session.
	Create(header Header) error
	// Append writes a durable session update record to an existing session log.
	Append(cwd string, sessionID acp.SessionId, rec UpdateRecord) error
	// Load reads a session header plus all persisted update records in file order.
	Load(cwd string, sessionID acp.SessionId) (Header, []UpdateRecord, error)
	// List returns persisted session IDs for the given working directory.
	List(cwd string) ([]acp.SessionId, error)
}

// Log is the single runtime owner for a session's durable state and replayable updates.
type Log struct {
	mu sync.RWMutex

	store Store

	header        Header
	updates       []acp.SessionUpdate
	configOptions []acp.SessionConfigOption
	modes         *acp.SessionModeState
	title         *string
	updatedAt     *string
}

// Logs manages cached session logs over a Store.
type Logs struct {
	mu    sync.RWMutex
	store Store
	logs  map[acp.SessionId]*Log
}

// NewLogs constructs a Logs owner over the provided Store.
func NewLogs(store Store) *Logs {
	return &Logs{
		store: store,
		logs:  make(map[acp.SessionId]*Log),
	}
}

// Get returns a loaded session if it is already in memory.
func (ls *Logs) Get(sessionID acp.SessionId) (*Log, bool) {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	log, ok := ls.logs[sessionID]
	return log, ok
}

// Create writes a new session header and returns the in-memory log.
func (ls *Logs) Create(header Header) (*Log, error) {
	if err := validateHeader(header); err != nil {
		return nil, err
	}
	if err := ls.store.Create(header); err != nil {
		return nil, err
	}
	log := newLog(ls.store, header)

	ls.mu.Lock()
	ls.logs[header.SessionId] = log
	ls.mu.Unlock()

	return log, nil
}

// Load returns the cached or persisted session log.
func (ls *Logs) Load(cwd string, sessionID acp.SessionId) (*Log, error) {
	if log, ok := ls.Get(sessionID); ok {
		return log, nil
	}

	log, err := ls.load(cwd, sessionID)
	if err != nil {
		return nil, err
	}

	ls.mu.Lock()
	defer ls.mu.Unlock()
	if cached, ok := ls.logs[sessionID]; ok {
		return cached, nil
	}
	ls.logs[sessionID] = log
	return log, nil
}

// Fork creates a new session header and copies the parent's persisted updates into it.
func (ls *Logs) Fork(cwd string, parentID acp.SessionId, header Header) (*Log, error) {
	parent, err := ls.Load(cwd, parentID)
	if err != nil {
		return nil, err
	}
	child, err := ls.Create(header)
	if err != nil {
		return nil, err
	}
	for _, update := range parent.Updates() {
		if err := child.Append(update); err != nil {
			return nil, err
		}
	}
	return child, nil
}

// List returns session info derived from persisted logs under cwd.
func (ls *Logs) List(cwd string) ([]acp.UnstableSessionInfo, error) {
	ids, err := ls.store.List(cwd)
	if err != nil {
		return nil, err
	}
	slices.Sort(ids)

	infos := make([]acp.UnstableSessionInfo, 0, len(ids))
	for _, sessionID := range ids {
		log, err := ls.Load(cwd, sessionID)
		if err != nil {
			return nil, err
		}
		infos = append(infos, log.Info())
	}
	return infos, nil
}

func (ls *Logs) load(cwd string, sessionID acp.SessionId) (*Log, error) {
	header, records, err := ls.store.Load(cwd, sessionID)
	if err != nil {
		return nil, err
	}
	log := newLog(ls.store, header)
	for _, rec := range records {
		log.applyUpdate(rec.Update)
	}
	return log, nil
}

// Header returns a copy of the durable session header.
func (l *Log) Header() Header {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return copyHeader(l.header)
}

// Info returns the current unstable session info.
func (l *Log) Info() acp.UnstableSessionInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var title *string
	if l.title != nil {
		value := *l.title
		title = &value
	}
	var updatedAt *string
	if l.updatedAt != nil {
		value := *l.updatedAt
		updatedAt = &value
	}

	return acp.UnstableSessionInfo{
		Cwd:       l.header.Cwd,
		SessionId: l.header.SessionId,
		Title:     title,
		UpdatedAt: updatedAt,
	}
}

// ConfigOptions returns the current full config option state.
func (l *Log) ConfigOptions() []acp.SessionConfigOption {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return copyConfigOptions(l.configOptions)
}

// Modes returns the current mode state.
func (l *Log) Modes() *acp.SessionModeState {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.modes == nil {
		return nil
	}
	modes := *l.modes
	modes.AvailableModes = slices.Clone(l.modes.AvailableModes)
	return &modes
}

// Models returns the header model state.
func (l *Log) Models() *acp.UnstableSessionModelState {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.header.Models == nil {
		return nil
	}
	models := *l.header.Models
	models.AvailableModels = slices.Clone(l.header.Models.AvailableModels)
	return &models
}

// Updates returns replayable session updates in file order.
func (l *Log) Updates() []acp.SessionUpdate {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.Clone(l.updates)
}

// Append persists and applies a session update.
func (l *Log) Append(update acp.SessionUpdate) error {
	rec := UpdateRecord{
		EventID:   uuid.NewString(),
		Timestamp: time.Now().UTC(),
		Update:    update,
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.store.Append(l.header.Cwd, l.header.SessionId, rec); err != nil {
		return err
	}
	l.applyUpdate(update)
	return nil
}

func (l *Log) applyUpdate(update acp.SessionUpdate) {
	if configUpdate := update.ConfigOptionUpdate; configUpdate != nil {
		l.configOptions = copyConfigOptions(configUpdate.ConfigOptions)
	}
	if modeUpdate := update.CurrentModeUpdate; modeUpdate != nil {
		if l.modes == nil {
			l.modes = &acp.SessionModeState{}
		}
		l.modes.CurrentModeId = modeUpdate.CurrentModeId
	}
	if infoUpdate := update.SessionInfoUpdate; infoUpdate != nil {
		if infoUpdate.Title == nil {
			l.title = nil
		} else {
			title := *infoUpdate.Title
			l.title = &title
		}
		if infoUpdate.UpdatedAt == nil {
			l.updatedAt = nil
		} else {
			updatedAt := *infoUpdate.UpdatedAt
			l.updatedAt = &updatedAt
		}
	}
	l.updates = append(l.updates, update)
}

func newLog(store Store, header Header) *Log {
	header = copyHeader(header)
	return &Log{
		store:         store,
		header:        header,
		configOptions: copyConfigOptions(header.ConfigOptions),
		modes:         copyModes(header.Modes),
		updatedAt:     acp.Ptr(header.Timestamp.Format(time.RFC3339Nano)),
	}
}

func copyHeader(header Header) Header {
	cloned := header
	cloned.Meta = maps.Clone(header.Meta)
	cloned.McpServers = slices.Clone(header.McpServers)
	cloned.ConfigOptions = copyConfigOptions(header.ConfigOptions)
	cloned.Modes = copyModes(header.Modes)
	if header.Models != nil {
		models := *header.Models
		models.AvailableModels = slices.Clone(header.Models.AvailableModels)
		cloned.Models = &models
	}
	if header.ParentSessionId != nil {
		parentSessionID := *header.ParentSessionId
		cloned.ParentSessionId = &parentSessionID
	}
	return cloned
}

func copyConfigOptions(options []acp.SessionConfigOption) []acp.SessionConfigOption {
	cloned := make([]acp.SessionConfigOption, len(options))
	for i, option := range options {
		cloned[i] = option
		if option.Select == nil {
			continue
		}
		selectOption := *option.Select
		if option.Select.Options.Ungrouped != nil {
			values := slices.Clone(*option.Select.Options.Ungrouped)
			selectOption.Options.Ungrouped = &values
		}
		if option.Select.Options.Grouped != nil {
			groups := slices.Clone(*option.Select.Options.Grouped)
			for j := range groups {
				groups[j].Options = slices.Clone(groups[j].Options)
			}
			selectOption.Options.Grouped = &groups
		}
		cloned[i].Select = &selectOption
	}
	return cloned
}

func copyModes(modes *acp.SessionModeState) *acp.SessionModeState {
	if modes == nil {
		return nil
	}
	cloned := *modes
	cloned.AvailableModes = slices.Clone(modes.AvailableModes)
	return &cloned
}

func validateHeader(header Header) error {
	switch {
	case header.EventID == "":
		return errors.New("event id is required")
	case header.Timestamp.IsZero():
		return errors.New("timestamp is required")
	case header.SessionEvent == "":
		return errors.New("session event is required")
	case header.SessionId == "":
		return errors.New("session id is required")
	case header.Cwd == "":
		return errors.New("cwd is required")
	default:
		return nil
	}
}
