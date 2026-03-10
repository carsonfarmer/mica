package main

import (
	"os"

	acp "github.com/coder/acp-go-sdk"

	"github.com/carsonfarmer/mica/pkg/agent"
	"github.com/carsonfarmer/mica/pkg/session"
	"github.com/carsonfarmer/mica/pkg/session/store"
)

func main() {
	fileStore := store.NewFileStore()
	logs := session.NewLogs(fileStore)
	ag := agent.New(logs)

	conn := acp.NewAgentSideConnection(ag, os.Stdout, os.Stdin)
	ag.SetAgentConnection(conn)

	<-conn.Done()
}
