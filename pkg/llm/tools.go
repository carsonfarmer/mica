package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	"github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
)

// Tool names.
const (
	ToolNameReadFile        = "read_file"
	ToolNameWriteFile       = "write_file"
	ToolNameExecuteCommand  = "execute_command"
	ToolNameTerminalCreate  = "terminal_create"
	ToolNameTerminalOutput  = "terminal_output"
	ToolNameTerminalWait    = "terminal_wait"
	ToolNameTerminalKill    = "terminal_kill"
	ToolNameTerminalRelease = "terminal_release"
	ToolNamePlan            = "plan"
)

// Context keys for passing runtime values to tools.
type ctxKey string

const (
	ctxClient    ctxKey = "acp-client"
	ctxSessionID ctxKey = "acp-session-id"
)

// WithClient returns a context carrying the ACP client.
func WithClient(ctx context.Context, c acp.Client) context.Context {
	return context.WithValue(ctx, ctxClient, c)
}

// WithSession returns a context carrying the session ID.
func WithSession(ctx context.Context, sid acp.SessionID) context.Context {
	return context.WithValue(ctx, ctxSessionID, sid)
}

// ClientFrom extracts the ACP client from context.
func ClientFrom(ctx context.Context) acp.Client {
	c, _ := ctx.Value(ctxClient).(acp.Client)
	return c
}

// SessionFrom extracts the session ID from context.
func SessionFrom(ctx context.Context) acp.SessionID {
	s, _ := ctx.Value(ctxSessionID).(acp.SessionID)
	return s
}

// ReadFileTool creates a tool that reads a file via the ACP client.
func ReadFileTool() fantasy.AgentTool {
	type input struct {
		Path  string `json:"path" description:"Absolute path to the file to read"`
		Line  int    `json:"line,omitempty" description:"Line number to start reading from (1-indexed)"`
		Limit int    `json:"limit,omitempty" description:"Maximum number of lines to read"`
	}
	return fantasy.NewParallelAgentTool(
		ToolNameReadFile,
		"Read a file from the local filesystem.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			resp, err := ClientFrom(ctx).ReadTextFile(ctx, &acp.ReadTextFileRequest{
				SessionID: SessionFrom(ctx),
				Path:      in.Path,
				Line:      in.Line,
				Limit:     in.Limit,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse(resp.Content), nil
		},
	)
}

// WriteFileTool creates a tool that writes a file via the ACP client.
func WriteFileTool() fantasy.AgentTool {
	type input struct {
		Path    string `json:"path" description:"Absolute path to the file to write"`
		Content string `json:"content" description:"Content to write to the file"`
	}
	return fantasy.NewParallelAgentTool(
		ToolNameWriteFile,
		"Write content to a file on the local filesystem.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			_, err := ClientFrom(ctx).WriteTextFile(ctx, &acp.WriteTextFileRequest{
				SessionID: SessionFrom(ctx),
				Path:      in.Path,
				Content:   in.Content,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse("File written successfully."), nil
		},
	)
}

// TerminalTool creates a combined terminal tool that handles full lifecycle.
func TerminalTool() fantasy.AgentTool {
	type input struct {
		Command string   `json:"command" description:"The shell command to execute"`
		Args    []string `json:"args,omitempty" description:"Command arguments"`
		Cwd     string   `json:"cwd,omitempty" description:"Working directory for the command"`
	}
	return fantasy.NewAgentTool(
		ToolNameExecuteCommand,
		"Execute a shell command on the local system.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
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

			exitResp, err := handle.WaitForExit(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			outResp, err := handle.CurrentOutput(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "%s", outResp.Output)
			if exitResp.ExitCode != nil {
				fmt.Fprintf(&b, "\n[Exit code: %d]", *exitResp.ExitCode)
			}
			if exitResp.Signal != "" {
				fmt.Fprintf(&b, "\n[Exited by signal: %s]", exitResp.Signal)
			}
			return fantasy.NewTextResponse(b.String()), nil
		},
	)
}

// TerminalCreateTool creates a new terminal and returns its ID.
func TerminalCreateTool() fantasy.AgentTool {
	type input struct {
		Command string   `json:"command" description:"The shell command to execute"`
		Args    []string `json:"args,omitempty" description:"Command arguments"`
		Cwd     string   `json:"cwd,omitempty" description:"Working directory for the command"`
	}
	return fantasy.NewAgentTool(
		ToolNameTerminalCreate,
		"Create a new terminal running a command. Returns the terminal ID.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			terminal, ok := client.(acp.ClientTerminal)
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
			return fantasy.NewTextResponse(resp.TerminalID), nil
		},
	)
}

// TerminalOutputTool gets the current output of a terminal.
func TerminalOutputTool() fantasy.AgentTool {
	type input struct {
		TerminalID string `json:"terminal_id" description:"The terminal ID from terminal_create"`
	}
	return fantasy.NewAgentTool(
		ToolNameTerminalOutput,
		"Get the current output of a terminal without waiting for exit.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			terminal, ok := client.(acp.ClientTerminal)
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
			return fantasy.NewTextResponse(out.Output), nil
		},
	)
}

// TerminalWaitTool waits for a terminal command to exit.
func TerminalWaitTool() fantasy.AgentTool {
	type input struct {
		TerminalID string `json:"terminal_id" description:"The terminal ID from terminal_create"`
	}
	return fantasy.NewAgentTool(
		ToolNameTerminalWait,
		"Wait for a terminal command to complete and return its exit status.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			terminal, ok := client.(acp.ClientTerminal)
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
			raw, _ := json.Marshal(exit)
			return fantasy.NewTextResponse(string(raw)), nil
		},
	)
}

// TerminalKillTool kills a terminal command.
func TerminalKillTool() fantasy.AgentTool {
	type input struct {
		TerminalID string `json:"terminal_id" description:"The terminal ID from terminal_create"`
	}
	return fantasy.NewAgentTool(
		ToolNameTerminalKill,
		"Kill a running terminal command without releasing it.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			terminal, ok := client.(acp.ClientTerminal)
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
			return fantasy.NewTextResponse("terminal killed"), nil
		},
	)
}

// TerminalReleaseTool kills a terminal command and frees its resources.
func TerminalReleaseTool() fantasy.AgentTool {
	type input struct {
		TerminalID string `json:"terminal_id" description:"The terminal ID from terminal_create"`
	}
	return fantasy.NewAgentTool(
		ToolNameTerminalRelease,
		"Release a terminal and free its resources. Kills the command if still running.",
		func(ctx context.Context, in input, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			client := ClientFrom(ctx)
			terminal, ok := client.(acp.ClientTerminal)
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
			return fantasy.NewTextResponse("terminal released"), nil
		},
	)
}

// PlanTool creates a plan tool for the model to declare its approach.
func PlanTool() fantasy.AgentTool {
	return fantasy.NewParallelAgentTool(
		ToolNamePlan,
		"Declare a plan or approach for the current task.",
		func(ctx context.Context, entries []acp.PlanEntry, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			stream := agentutil.NewSessionStream(ClientFrom(ctx), SessionFrom(ctx))
			if err := stream.SendPlan(ctx, entries); err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			return fantasy.NewTextResponse("Plan accepted."), nil
		},
	)
}
