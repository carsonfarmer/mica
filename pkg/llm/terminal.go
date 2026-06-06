package llm

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
)

const (
	ToolNameExecute         = "execute"
	ToolNameTerminalCreate  = "terminal_create"
	ToolNameTerminalOutput  = "terminal_output"
	ToolNameTerminalWait    = "terminal_wait"
	ToolNameTerminalKill    = "terminal_kill"
	ToolNameTerminalRelease = "terminal_release"
)

type TerminalInput struct {
	Command string   `json:"command" description:"The shell command to execute"`
	Args    []string `json:"args,omitempty" description:"Command arguments"`
	Cwd     string   `json:"cwd,omitempty" description:"Working directory for the command"`
}

// TerminalTool creates a combined terminal tool.
func TerminalTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameExecute,
		"Execute a shell command on the local system.",
		func(ctx context.Context, in TerminalInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			terminal, ok := client.(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}

			handle, err := agentutil.CreateTerminalHandle(ctx, terminal, &acp.CreateTerminalRequest{
				SessionID: SessionFrom(ctx),
				Command:   in.Command,
				Args:      in.Args,
				CWD:       in.Cwd,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			defer handle.Release(ctx)

			stream := StreamFrom(ctx)
			stream.SendUpdate(ctx, acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithTitle(strings.TrimSpace(in.Command+" "+strings.Join(in.Args, " "))),
				acp.WithToolContent(acp.ToolTerminal(handle.ID)),
			))

			exitResp, err := handle.WaitForExit(ctx)
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}

			outResp, err := handle.CurrentOutput(ctx)
			if err != nil {
				return ToolFailedResponse(tc, err), nil
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

			final := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithTitle(strings.TrimSpace(in.Command+" "+strings.Join(in.Args, " "))),
				acp.WithRawOutput(msg.String()),
			)
			return ToolResponse(msg.String(), final), nil
		},
	)
}

// TerminalCreateTool creates a new terminal and returns its ID.
func TerminalCreateTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalCreate,
		"Create a new terminal running a command. Returns the terminal ID.",
		func(ctx context.Context, in TerminalInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			resp, err := terminal.CreateTerminal(ctx, &acp.CreateTerminalRequest{
				SessionID: SessionFrom(ctx),
				Command:   in.Command,
				Args:      in.Args,
				CWD:       in.Cwd,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			msg := fmt.Sprintf("terminal %s created", resp.TerminalID)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithTitle(msg),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithRawOutput(msg),
			)
			return ToolResponse(msg, upd), nil
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
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			out, err := terminal.TerminalOutput(ctx, &acp.TerminalOutputRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
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

			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithRawOutput(msg.String()),
			)
			return ToolResponse(msg.String(), upd), nil
		},
	)
}

// TerminalWaitTool waits for a terminal command to exit.
func TerminalWaitTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalWait,
		"Wait for a terminal command to complete and return its exit status.",
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			exit, err := terminal.WaitForTerminalExit(ctx, &acp.WaitForTerminalExitRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			var msg string
			if exit.ExitCode != nil {
				msg = fmt.Sprintf("exit code %d", *exit.ExitCode)
			} else {
				msg = fmt.Sprintf("killed by signal %s", exit.Signal)
			}
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithRawOutput(msg),
			)
			return ToolResponse(msg, upd), nil
		},
	)
}

// TerminalKillTool kills a terminal command.
func TerminalKillTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalKill,
		"Kill a running terminal command without releasing it.",
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			_, err := terminal.KillTerminal(ctx, &acp.KillTerminalRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			msg := fmt.Sprintf("terminal %s killed", in.TerminalID)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithRawOutput(msg),
			)
			return ToolResponse(msg, upd), nil
		},
	)
}

// TerminalReleaseTool kills a terminal command and frees its resources.
func TerminalReleaseTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ToolNameTerminalRelease,
		"Release a terminal and free its resources. Kills the command if still running.",
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			_, err := terminal.ReleaseTerminal(ctx, &acp.ReleaseTerminalRequest{
				SessionID:  SessionFrom(ctx),
				TerminalID: in.TerminalID,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			msg := fmt.Sprintf("terminal %s released", in.TerminalID)
			upd := acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithStatus(acp.ToolCompleted),
				acp.WithRawOutput(msg),
			)
			return ToolResponse(msg, upd), nil
		},
	)
}
