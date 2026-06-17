package tools

import (
	"context"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/mica/pkg/core"
)

type TerminalInput struct {
	Command string   `json:"command" description:"The program to run (e.g. 'ls', 'go', 'git'). Do NOT include arguments here."`
	Args    []string `json:"args,omitempty" description:"Arguments to pass to the command, one per element (e.g. ['-la', '/tmp'])."`
	Cwd     string   `json:"cwd,omitempty" description:"Working directory for the command"`
}

// TerminalTool executes a shell command, streams a live terminal widget,
// blocks until exit, then returns output and exit status.
func TerminalTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"execute",
		"Execute a shell command. Use 'command' for the program name and 'args' for its arguments (e.g. command='ls', args=['-la', '/tmp']). Do not put flags or arguments in the command field.",
		func(ctx context.Context, in TerminalInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if err := CheckPermission(ctx, tc, "execute:"+in.Command); err != nil {
				return ToolErrorResponse(err.Error()), nil
			}
			client, ok := core.ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			sess := core.SessionFrom(ctx)
			handle, err := agentutil.CreateTerminalHandle(ctx, client, &acp.CreateTerminalRequest{
				SessionID: sess.SessionID,
				Command:   in.Command,
				Args:      in.Args,
				CWD:       in.Cwd,
			})
			if err != nil {
				return ToolFailedResponse(tc, err), nil
			}
			defer handle.Release(ctx)

			stream := agentutil.NewSessionStream(core.ClientFrom(ctx), sess.SessionID)
			title := strings.TrimSpace(in.Command + " " + strings.Join(in.Args, " "))
			stream.SendUpdate(ctx, acp.UpdateToolCallDelta(
				acp.ToolCallID(tc.ID),
				acp.WithTitle(title),
				acp.WithToolContent(acp.ToolTerminal(handle.ID)),
			))

			if exitResp, err := handle.WaitForExit(ctx); err != nil {
				return ToolFailedResponse(tc, err), nil
			} else if outResp, err := handle.CurrentOutput(ctx); err != nil {
				return ToolFailedResponse(tc, err), nil
			} else {
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
					acp.WithTitle(title),
					acp.WithRawOutput(msg.String()),
				)
				return ToolResponse(msg.String(), final), nil
			}
		},
	)
}

func TerminalCreateTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"terminal_create",
		"Create a new terminal running a command. Returns the terminal ID.",
		func(ctx context.Context, in TerminalInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := core.ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			if resp, err := terminal.CreateTerminal(ctx, &acp.CreateTerminalRequest{
				SessionID: core.SessionFrom(ctx).SessionID,
				Command:   in.Command,
				Args:      in.Args,
				CWD:       in.Cwd,
			}); err != nil {
				return ToolFailedResponse(tc, err), nil
			} else {
				msg := fmt.Sprintf("terminal %s created", resp.TerminalID)
				upd := acp.UpdateToolCallDelta(
					acp.ToolCallID(tc.ID),
					acp.WithTitle(msg),
					acp.WithStatus(acp.ToolCompleted),
					acp.WithRawOutput(msg),
				)
				return ToolResponse(msg, upd), nil
			}
		},
	)
}

type TerminalIDInput struct {
	TerminalID string `json:"terminal_id" description:"The terminal ID from terminal_create"`
}

func TerminalOutputTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"terminal_output",
		"Get the current output of a terminal without waiting for exit.",
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := core.ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			if out, err := terminal.TerminalOutput(ctx, &acp.TerminalOutputRequest{
				SessionID:  core.SessionFrom(ctx).SessionID,
				TerminalID: in.TerminalID,
			}); err != nil {
				return ToolFailedResponse(tc, err), nil
			} else {
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
			}
		},
	)
}

func TerminalWaitTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"terminal_wait",
		"Wait for a terminal command to complete and return its exit status.",
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := core.ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			if exit, err := terminal.WaitForTerminalExit(ctx, &acp.WaitForTerminalExitRequest{
				SessionID:  core.SessionFrom(ctx).SessionID,
				TerminalID: in.TerminalID,
			}); err != nil {
				return ToolFailedResponse(tc, err), nil
			} else {
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
			}
		},
	)
}

func TerminalKillTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"terminal_kill",
		"Kill a running terminal command without releasing it.",
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := core.ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			if _, err := terminal.KillTerminal(ctx, &acp.KillTerminalRequest{
				SessionID:  core.SessionFrom(ctx).SessionID,
				TerminalID: in.TerminalID,
			}); err != nil {
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

func TerminalReleaseTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"terminal_release",
		"Release a terminal and free its resources. Kills the command if still running.",
		func(ctx context.Context, in TerminalIDInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			terminal, ok := core.ClientFrom(ctx).(acp.ClientTerminal)
			if !ok {
				return ToolFailedResponse(tc, fmt.Errorf("terminal capability not available")), nil
			}
			if _, err := terminal.ReleaseTerminal(ctx, &acp.ReleaseTerminalRequest{
				SessionID:  core.SessionFrom(ctx).SessionID,
				TerminalID: in.TerminalID,
			}); err != nil {
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
