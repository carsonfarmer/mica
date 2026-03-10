package app

import "path/filepath"

const (
	ProjectName             = "Mica"
	BinaryName              = "mica"
	AgentName               = "mica-echo"
	AgentTitle              = "Mica Echo Agent"
	LogDirName              = ".mica"
	SessionsDirName         = "sessions"
	LogFileExt              = ".jsonl"
	SessionEventNew         = "new"
	SessionEventFork        = "fork"
	DefaultSessionModeID    = "default"
	DefaultSessionModeName  = "Default"
	DefaultResponseFormatID = "echo"
	RawResponseFormatID     = "raw"
	DefaultModelID          = "echo-v1"
	DefaultModelName        = "Echo v1"
)

var Version = "dev"

func SessionsDir(root string) string {
	return filepath.Join(root, LogDirName, SessionsDirName)
}

func SessionLogFile(root string, sessionID string) string {
	return filepath.Join(SessionsDir(root), sessionID+LogFileExt)
}
