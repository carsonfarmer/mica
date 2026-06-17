package agent

import (
	"context"
	"strings"

	acp "github.com/carsonfarmer/go-acp-sdk"
)

// Command wraps an ACP AvailableCommand with an Execute handler.
// Execute receives context (carrying Client/Session) and args.
// A nil prompt means the command was fully handled; non-nil
// replaces the user prompt (e.g., skill expansion).
type Command struct {
	acp.AvailableCommand
	Execute func(ctx context.Context, args string) ([]acp.ContentBlock, error)
}

// interceptCommand returns (prompt, error). Original prompt → no match;
// nil prompt → handled (caller returns StopEndTurn); replacement prompt →
// continue to LLM; error → RPC error.
func (a *Agent) interceptCommand(ctx context.Context, prompt []acp.ContentBlock) ([]acp.ContentBlock, error) {
	text := ""
	for _, b := range prompt {
		if b.Text != nil {
			text = b.Text.Text
			break
		}
	}
	if !strings.HasPrefix(text, "/") {
		return prompt, nil
	}
	name, args, _ := strings.Cut(text[1:], " ")
	cmd, ok := a.commands[strings.TrimSpace(name)]
	if !ok {
		return prompt, nil
	}
	return cmd.Execute(ctx, strings.TrimSpace(args))
}

func (a *Agent) GetAvailableCommands() []acp.AvailableCommand {
	cmds := make([]acp.AvailableCommand, 0, len(a.commands))
	for _, c := range a.commands {
		cmds = append(cmds, c.AvailableCommand)
	}
	return cmds
}
