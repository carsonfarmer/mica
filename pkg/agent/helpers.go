package agent

import (
	"fmt"
	"os"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/google/uuid"

	"github.com/carsonfarmer/mica/internal/app"
	"github.com/carsonfarmer/mica/pkg/session"
)

func newSessionHeader(req acp.NewSessionRequest) session.Header {
	state := defaultConfigState()
	return session.Header{
		EventID:       uuid.NewString(),
		Timestamp:     time.Now().UTC(),
		SessionEvent:  app.SessionEventNew,
		Cwd:           req.Cwd,
		McpServers:    append([]acp.McpServer(nil), req.McpServers...),
		Meta:          req.Meta,
		ConfigOptions: configOptions(state),
		Modes:         modeState(state.ModeID),
		Models:        unstableModelState(state.ModelID),
		SessionId:     acp.SessionId(uuid.NewString()),
	}
}

func forkSessionHeader(req acp.UnstableForkSessionRequest, state configState, modes *acp.SessionModeState) session.Header {
	return session.Header{
		EventID:         uuid.NewString(),
		Timestamp:       time.Now().UTC(),
		SessionEvent:    app.SessionEventFork,
		Cwd:             req.Cwd,
		McpServers:      append([]acp.McpServer(nil), req.McpServers...),
		ConfigOptions:   configOptions(state),
		Modes:           modes,
		Models:          unstableModelState(state.ModelID),
		SessionId:       acp.SessionId(uuid.NewString()),
		ParentSessionId: acp.Ptr(req.SessionId),
	}
}

func newSessionResponse(log *session.Log) acp.NewSessionResponse {
	state := configStateFromLog(log)
	return acp.NewSessionResponse{
		SessionId:     log.Header().SessionId,
		ConfigOptions: configOptions(state),
		Modes:         modeState(state.ModeID),
		Models:        stableModelState(state.ModelID),
	}
}

func loadSessionResponse(log *session.Log) acp.LoadSessionResponse {
	state := configStateFromLog(log)
	return acp.LoadSessionResponse{
		ConfigOptions: configOptions(state),
		Modes:         modeState(state.ModeID),
		Models:        stableModelState(state.ModelID),
	}
}

func resumeSessionResponse(log *session.Log) acp.UnstableResumeSessionResponse {
	state := configStateFromLog(log)
	return acp.UnstableResumeSessionResponse{
		ConfigOptions: configOptions(state),
		Modes:         modeState(state.ModeID),
		Models:        unstableModelState(state.ModelID),
	}
}

func forkSessionResponse(log *session.Log) acp.UnstableForkSessionResponse {
	state := configStateFromLog(log)
	return acp.UnstableForkSessionResponse{
		SessionId:     log.Header().SessionId,
		ConfigOptions: configOptions(state),
		Modes:         modeState(state.ModeID),
		Models:        unstableModelState(state.ModelID),
	}
}

func sessionInfoUpdate(log *session.Log, prompt string) acp.SessionUpdate {
	title := log.Info().Title
	if title == nil {
		title = deriveTitle(prompt)
	}
	return acp.SessionUpdate{
		SessionInfoUpdate: &acp.SessionSessionInfoUpdate{
			Title:     title,
			UpdatedAt: acp.Ptr(time.Now().UTC().Format(time.RFC3339Nano)),
		},
	}
}

func deriveTitle(text string) *string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) > 40 {
		runes = runes[:40]
	}
	title := string(runes)
	return &title
}

func listCWD(cwd *string) (string, error) {
	if cwd != nil && *cwd != "" {
		return *cwd, nil
	}
	current, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return current, nil
}

func flattenPrompt(prompt []acp.ContentBlock) string {
	parts := make([]string, 0, len(prompt))
	for _, block := range prompt {
		switch {
		case block.Text != nil:
			parts = append(parts, block.Text.Text)
		case block.ResourceLink != nil:
			if block.ResourceLink.Name != "" {
				parts = append(parts, block.ResourceLink.Name+" "+block.ResourceLink.Uri)
				continue
			}
			parts = append(parts, block.ResourceLink.Uri)
		}
	}
	return strings.Join(parts, "\n")
}

func splitIntoChunks(text string, chunkSize int) []string {
	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)+chunkSize-1)/chunkSize)
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
