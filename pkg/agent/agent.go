// Package agent provides the ACP protocol adapter layer.
package agent

import (
	"context"
	"errors"
	"os"
	"sync"

	acp "github.com/coder/acp-go-sdk"

	"github.com/carsonfarmer/mica/internal/app"
	"github.com/carsonfarmer/mica/pkg/session"
)

const (
	updateChunkSize = 100
)

// Agent implements the ACP agent interfaces for the Phase 1 echo scaffold.
type Agent struct {
	conn *acp.AgentSideConnection
	logs *session.Logs

	mu      sync.Mutex
	cancels map[acp.SessionId]context.CancelFunc
}

// New constructs an Agent over the session owner.
func New(logs *session.Logs) *Agent {
	return &Agent{
		logs:    logs,
		cancels: make(map[acp.SessionId]context.CancelFunc),
	}
}

// SetAgentConnection implements acp.AgentConnAware.
func (a *Agent) SetAgentConnection(conn *acp.AgentSideConnection) {
	a.conn = conn
}

// Authenticate implements acp.Agent.
func (a *Agent) Authenticate(ctx context.Context, req acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

// Initialize implements acp.Agent.
func (a *Agent) Initialize(ctx context.Context, req acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		AgentInfo: &acp.Implementation{
			Name:    app.AgentName,
			Title:   acp.Ptr(app.AgentTitle),
			Version: app.Version,
		},
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: true,
			SessionCapabilities: acp.SessionCapabilities{
				Fork:   &acp.SessionForkCapabilities{},
				List:   &acp.SessionListCapabilities{},
				Resume: &acp.SessionResumeCapabilities{},
			},
		},
		AuthMethods:     []acp.AuthMethod{},
		ProtocolVersion: acp.ProtocolVersionNumber,
	}, nil
}

// NewSession implements acp.Agent.
func (a *Agent) NewSession(ctx context.Context, req acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	log, err := a.logs.Create(newSessionHeader(req))
	if err != nil {
		return acp.NewSessionResponse{}, err
	}
	return newSessionResponse(log), nil
}

// Prompt implements acp.Agent.
func (a *Agent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	log, err := a.getLog(req.SessionId)
	if err != nil {
		return acp.PromptResponse{}, err
	}

	promptCtx, cancel := context.WithCancel(ctx)
	a.setCancel(req.SessionId, cancel)
	defer a.clearCancel(req.SessionId)

	promptText := flattenPrompt(req.Prompt)
	if err := a.appendAndNotify(ctx, req.SessionId, log, sessionInfoUpdate(log, promptText)); err != nil {
		return acp.PromptResponse{}, err
	}
	userUpdate := acp.UpdateUserMessageText(promptText)
	if err := a.appendAndNotify(ctx, req.SessionId, log, userUpdate); err != nil {
		return acp.PromptResponse{}, err
	}

	state := configStateFromLog(log)
	responseText := state.formatResponse(promptText)

	for _, chunk := range splitIntoChunks(responseText, updateChunkSize) {
		select {
		case <-promptCtx.Done():
			return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
		default:
		}

		update := acp.UpdateAgentMessageText(chunk)
		if err := a.appendAndNotify(ctx, req.SessionId, log, update); err != nil {
			return acp.PromptResponse{}, err
		}
	}

	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

// Cancel implements acp.Agent.
func (a *Agent) Cancel(ctx context.Context, req acp.CancelNotification) error {
	a.mu.Lock()
	cancel := a.cancels[req.SessionId]
	delete(a.cancels, req.SessionId)
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// SetSessionConfigOption implements acp.Agent.
func (a *Agent) SetSessionConfigOption(ctx context.Context, req acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	log, err := a.getLog(req.SessionId)
	if err != nil {
		return acp.SetSessionConfigOptionResponse{}, err
	}

	state, modeChanged, err := applyConfigRequest(configStateFromLog(log), req)
	if err != nil {
		return acp.SetSessionConfigOptionResponse{}, err
	}

	options := configOptions(state)
	configUpdate := acp.SessionUpdate{
		ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
			ConfigOptions: options,
		},
	}
	if err := a.appendAndNotify(ctx, req.SessionId, log, configUpdate); err != nil {
		return acp.SetSessionConfigOptionResponse{}, err
	}

	if modeChanged {
		modeUpdate := acp.SessionUpdate{
			CurrentModeUpdate: &acp.SessionCurrentModeUpdate{
				CurrentModeId: state.ModeID,
			},
		}
		if err := a.appendAndNotify(ctx, req.SessionId, log, modeUpdate); err != nil {
			return acp.SetSessionConfigOptionResponse{}, err
		}
	}

	return acp.SetSessionConfigOptionResponse{ConfigOptions: options}, nil
}

// SetSessionMode implements acp.Agent.
func (a *Agent) SetSessionMode(ctx context.Context, req acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionSetMode)
}

// LoadSession implements acp.AgentLoader.
func (a *Agent) LoadSession(ctx context.Context, req acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	log, err := a.loadLog(req.Cwd, req.SessionId)
	if err != nil {
		return acp.LoadSessionResponse{}, err
	}
	for _, update := range log.Updates() {
		if err := a.notify(ctx, req.SessionId, update); err != nil {
			return acp.LoadSessionResponse{}, err
		}
	}
	return loadSessionResponse(log), nil
}

// UnstableForkSession implements acp.AgentExperimental.
func (a *Agent) UnstableForkSession(ctx context.Context, req acp.UnstableForkSessionRequest) (acp.UnstableForkSessionResponse, error) {
	parent, err := a.loadLog(req.Cwd, req.SessionId)
	if err != nil {
		return acp.UnstableForkSessionResponse{}, err
	}
	state := configStateFromLog(parent)

	child, err := a.logs.Fork(req.Cwd, req.SessionId, forkSessionHeader(req, state, parent.Modes()))
	if err != nil {
		return acp.UnstableForkSessionResponse{}, err
	}
	return forkSessionResponse(child), nil
}

// UnstableListSessions implements acp.AgentExperimental.
func (a *Agent) UnstableListSessions(ctx context.Context, req acp.UnstableListSessionsRequest) (acp.UnstableListSessionsResponse, error) {
	cwd, err := listCWD(req.Cwd)
	if err != nil {
		return acp.UnstableListSessionsResponse{}, err
	}
	sessions, err := a.logs.List(cwd)
	if err != nil {
		return acp.UnstableListSessionsResponse{}, err
	}
	return acp.UnstableListSessionsResponse{Sessions: sessions}, nil
}

// UnstableResumeSession implements acp.AgentExperimental.
func (a *Agent) UnstableResumeSession(ctx context.Context, req acp.UnstableResumeSessionRequest) (acp.UnstableResumeSessionResponse, error) {
	log, err := a.loadLog(req.Cwd, req.SessionId)
	if err != nil {
		return acp.UnstableResumeSessionResponse{}, err
	}
	return resumeSessionResponse(log), nil
}

// UnstableSetSessionModel implements acp.AgentExperimental.
func (a *Agent) UnstableSetSessionModel(ctx context.Context, req acp.UnstableSetSessionModelRequest) (acp.UnstableSetSessionModelResponse, error) {
	return acp.UnstableSetSessionModelResponse{}, acp.NewMethodNotFound(acp.AgentMethodSessionSetModel)
}

func (a *Agent) getLog(sessionID acp.SessionId) (*session.Log, error) {
	log, ok := a.logs.Get(sessionID)
	if !ok {
		return nil, acp.NewInvalidParams(map[string]string{"sessionId": string(sessionID)})
	}
	return log, nil
}

func (a *Agent) loadLog(cwd string, sessionID acp.SessionId) (*session.Log, error) {
	log, err := a.logs.Load(cwd, sessionID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, acp.NewInvalidParams(map[string]string{"sessionId": string(sessionID)})
		}
		return nil, err
	}
	return log, nil
}

func (a *Agent) appendAndNotify(ctx context.Context, sessionID acp.SessionId, log *session.Log, update acp.SessionUpdate) error {
	if err := log.Append(update); err != nil {
		return err
	}
	return a.notify(ctx, sessionID, update)
}

func (a *Agent) notify(ctx context.Context, sessionID acp.SessionId, update acp.SessionUpdate) error {
	if a.conn == nil {
		return nil
	}
	return a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sessionID,
		Update:    update,
	})
}

func (a *Agent) setCancel(sessionID acp.SessionId, cancel context.CancelFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cancels[sessionID] = cancel
}

func (a *Agent) clearCancel(sessionID acp.SessionId) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.cancels, sessionID)
}
