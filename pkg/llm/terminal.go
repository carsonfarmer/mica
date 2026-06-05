package llm

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
)

// Tool name constants for terminal tools.
const (
	ToolNameExecuteCommand  = "execute_command"
	ToolNameTerminalCreate  = "terminal_create"
	ToolNameTerminalOutput  = "terminal_output"
	ToolNameTerminalWait    = "terminal_wait"
	ToolNameTerminalKill    = "terminal_kill"
	ToolNameTerminalRelease = "terminal_release"
)

// TerminalInput is the input for terminal tools.
type TerminalInput struct {
	Command string   `json:"command" description:"The shell command to execute"`
	Args    []string `json:"args,omitempty" description:"Command arguments"`
	Cwd     string   `json:"cwd,omitempty" description:"Working directory for the command"`
}

// TerminalTool creates a combined terminal tool.
func TerminalTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameExecuteCommand,
		"Execute a shell command on the local system.",
		func(ctx context.Context, in TerminalInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			terminal, ok := client.(acp.ClientTerminal)
			if !ok {
				return fantasy.NewTextErrorResponse("terminal capability not available"), nil
			}

			handle, err := agentutil.CreateTerminalHandle(ctx, terminal, &acp.CreateTerminalRequest{
				SessionID: SessionFrom(ctx),
				Command:   in.Command,
				Args:      in.Args,
				CWD:       in.Cwd,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			defer handle.Release(ctx)

			sid := SessionFrom(ctx)
			upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID))
			upd.ToolCallUpdate.Title = strings.TrimSpace(in.Command + " " + strings.Join(in.Args, " "))
			upd.ToolCallUpdate.Content = []acp.ToolCallContent{acp.ToolTerminal(handle.ID)}
			client.SessionUpdate(ctx, &acp.SessionNotification{SessionID: sid, Update: upd})

			exitResp, err := handle.WaitForExit(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			outResp, err := handle.CurrentOutput(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var msg strings.Builder
			msg.WriteString(outResp.Output)
			if outResp.Truncated {
				msg.WriteString("\n[output truncated]")
			}
			if exitResp.ExitCode != nil {
				fmt.Fprintf(&msg, "\nexit code %d", *exitResp.ExitCode)
			} else if exitResp.Signal != "" {
				fmt.Fprintf(&msg, "\nkilled by signal %s", exitResp.Signal)
			}
			return fantasy.NewTextResponse(msg.String()), nil
		},
	)
}

// TerminalCreateTool creates a new terminal and returns its ID.
func TerminalCreateTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalCreate,
		"Create a new terminal running a command. Returns the terminal ID.",
		func(ctx context.Context, in TerminalInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return fantasy.NewTextErrorResponse("terminal capability not available"), nil
			}
			resp, err := terminal.CreateTerminal(ctx, &acp.CreateTerminalRequest{
				SessionID: SessionFrom(ctx),
				Command:   in.Command,
				Args:      in.Args,
				CWD:       in.Cwd,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(fmt.Sprintf("terminal %s created", resp.TerminalID)), nil
		},
	)
}

type TerminalIDInput struct {
	TerminalID string `json:"terminal_id" description:"The terminal ID from terminal_create"`
}

// TerminalOutputTool gets the current output of a terminal.
func TerminalOutputTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalOutput,
		"Get the current output of a terminal without waiting for exit.",
		func(ctx context.Context, in TerminalIDInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return fantasy.NewTextErrorResponse("terminal capability not available"), nil
			}
			out, err := terminal.TerminalOutput(ctx, &acp.TerminalOutputRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var msg strings.Builder
			msg.WriteString(out.Output)
			if out.Truncated {
				msg.WriteString("\n[output truncated]")
			}
			if out.ExitStatus != nil {
				if out.ExitStatus.ExitCode != nil {
					fmt.Fprintf(&msg, "\nexit code %d", *out.ExitStatus.ExitCode)
				} else if out.ExitStatus.Signal != "" {
					fmt.Fprintf(&msg, "\nkilled by signal %s", out.ExitStatus.Signal)
				}
			}
			return fantasy.NewTextResponse(msg.String()), nil
		},
	)
}

// TerminalWaitTool waits for a terminal command to exit.
func TerminalWaitTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalWait,
		"Wait for a terminal command to complete and return its exit status.",
		func(ctx context.Context, in TerminalIDInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return fantasy.NewTextErrorResponse("terminal capability not available"), nil
			}
			exit, err := terminal.WaitForTerminalExit(ctx, &acp.WaitForTerminalExitRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if exit.ExitCode != nil {
				return fantasy.NewTextResponse(fmt.Sprintf("exit code %d", *exit.ExitCode)), nil
			}
			return fantasy.NewTextResponse(fmt.Sprintf("killed by signal %s", exit.Signal)), nil
		},
	)
}

// TerminalKillTool kills a terminal command.
func TerminalKillTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalKill,
		"Kill a running terminal command without releasing it.",
		func(ctx context.Context, in TerminalIDInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return fantasy.NewTextErrorResponse("terminal capability not available"), nil
			}
			_, err := terminal.KillTerminal(ctx, &acp.KillTerminalRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(fmt.Sprintf("terminal %s killed", in.TerminalID)), nil
		},
	)
}

// TerminalReleaseTool kills a terminal command and frees its resources.
func TerminalReleaseTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalRelease,
		"Release a terminal and free its resources. Kills the command if still running.",
		func(ctx context.Context, in TerminalIDInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return fantasy.NewTextErrorResponse("terminal capability not available"), nil
			}
			_, err := terminal.ReleaseTerminal(ctx, &acp.ReleaseTerminalRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(fmt.Sprintf("terminal %s released", in.TerminalID)), nil
		},
	)
}
