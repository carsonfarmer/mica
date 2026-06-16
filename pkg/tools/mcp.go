package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/fantasy"
	acp "github.com/carsonfarmer/go-acp-sdk"
	acpmcp "github.com/carsonfarmer/go-acp-sdk/mcp"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/carsonfarmer/mica/pkg/core"
)

// McpCatalogHook builds a PromptHook that appends the MCP tools catalog
// to the system prompt (for future use with progressive disclosure).
func McpCatalogHook(mgr *acpmcp.Manager) core.PromptHook {
	return func(b *strings.Builder) {
		servers := mgr.Servers()
		if len(servers) == 0 {
			return
		}
		fmt.Fprint(b, "\n## MCP Tools\n")
		for _, srv := range servers {
			tools, err := mgr.ListTools(srv)
			if err != nil || len(tools) == 0 {
				continue
			}
			fmt.Fprintf(b, "\n### %s\n", srv)
			for _, t := range tools {
				fmt.Fprintf(b, "- **%s** — %s\n", t.Name, t.Description)
			}
		}
	}
}

type McpInput struct {
	Server   string          `json:"server,omitempty" description:"List tools for this server."`
	Tool     string          `json:"tool,omitempty" description:"Call this tool (requires server)."`
	Describe string          `json:"describe,omitempty" description:"Get JSON schema for this tool (requires server)."`
	Args     json.RawMessage `json:"args,omitempty" description:"Arguments for the tool call (JSON)."`
}

func MCPTool() fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"mcp",
		"MCP gateway — call tools on connected MCP servers.\n\n"+
			"Usage:\n"+
			"  mcp({ })                              → Show connected servers\n"+
			"  mcp({ server: \"name\" })               → List tools (name + description)\n"+
			"  mcp({ server: \"name\", describe: \"t\" }) → Show tool's full parameters\n"+
			"  mcp({ server: \"name\", tool: \"t\", args: '{\"key\":\"value\"}' }) → Execute\n",
		func(ctx context.Context, in McpInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sess := core.SessionFrom(ctx)
			if sess == nil || sess.MCP == nil {
				return ToolFailedResponse(tc, fmt.Errorf("no MCP manager")), nil
			}
			switch {
			case in.Tool != "":
				return handleMCPCall(ctx, sess.MCP, in, tc)
			case in.Describe != "":
				return handleMCPDescribe(sess.MCP, in, tc)
			case in.Server != "":
				return handleMCPList(sess.MCP, in, tc)
			default:
				return handleMCPStatus(sess.MCP, tc)
			}
		},
	)
}

func handleMCPStatus(mgr *acpmcp.Manager, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
	msg := "No MCP servers connected."
	if s := mgr.Servers(); len(s) > 0 {
		msg = "Connected MCP servers: " + strings.Join(s, ", ")
	}
	upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID), acp.WithStatus(acp.ToolCompleted), acp.WithTitle("mcp"), acp.WithRawOutput(msg))
	return ToolResponse(msg, upd), nil
}

func handleMCPList(mgr *acpmcp.Manager, in McpInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
	tools, err := mgr.ListTools(in.Server)
	if err != nil {
		return ToolFailedResponse(tc, err), nil
	}
	var b strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&b, "- %s: %s\n", t.Name, t.Description)
	}
	msg := b.String()
	upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID), acp.WithStatus(acp.ToolCompleted), acp.WithTitle("mcp list "+in.Server), acp.WithRawOutput(msg))
	return ToolResponse(msg, upd), nil
}

func handleMCPDescribe(mgr *acpmcp.Manager, in McpInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
	t, err := mgr.DescribeTool(in.Server, in.Describe)
	if err != nil {
		return ToolFailedResponse(tc, err), nil
	}
	schema, _ := json.MarshalIndent(t.InputSchema, "", "  ")
	msg := fmt.Sprintf("%s: %s\n\nParameters:\n%s", t.Name, t.Description, string(schema))
	upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID), acp.WithStatus(acp.ToolCompleted), acp.WithTitle("mcp describe "+in.Describe), acp.WithRawOutput(msg))
	return ToolResponse(msg, upd), nil
}

func handleMCPCall(ctx context.Context, mgr *acpmcp.Manager, in McpInput, tc fantasy.ToolCall) (fantasy.ToolResponse, error) {
	var args map[string]any
	if len(in.Args) > 0 {
		if err := json.Unmarshal(in.Args, &args); err != nil {
			return ToolFailedResponse(tc, fmt.Errorf("invalid args JSON: %w", err)), nil
		}
	}
	result, err := mgr.CallTool(ctx, in.Server, in.Tool, args)
	if err != nil {
		return ToolFailedResponse(tc, err), nil
	}
	var out strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			out.WriteString(tc.Text)
		}
	}
	if out.Len() == 0 {
		out.WriteString("(no text output)")
	}
	msg := out.String()
	upd := acp.UpdateToolCallDelta(acp.ToolCallID(tc.ID), acp.WithStatus(acp.ToolCompleted), acp.WithTitle("mcp "+in.Server+"/"+in.Tool), acp.WithRawOutput(msg))
	return ToolResponse(msg, upd), nil
}
