package agent

import (
	"context"

	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/agentutil"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/mica/pkg/core"
	"github.com/carsonfarmer/mica/pkg/tools"
)

// CompactCommand triggers conversation compaction via the compact tool's logic.
func CompactCommand(store storage.Store[*core.AgentSession], reg *core.Registry) Command {
	return Command{
		AvailableCommand: acp.NewAvailableCommand("compact", "Summarize conversation history", "[instructions]"),
		Execute: func(ctx context.Context, args string) ([]acp.ContentBlock, error) {
			result, err := tools.Compact(ctx, store, reg, args)
			if err != nil {
				return nil, err
			}
			sess := core.SessionFrom(ctx)
			agentutil.NewSessionStream(core.ClientFrom(ctx), sess.SessionID).SendText(ctx, result)
			return nil, nil
		},
	}
}

// TitleCommand sets the session title, or shows current info if called with no args.
func TitleCommand(store storage.Store[*core.AgentSession]) Command {
	return Command{
		AvailableCommand: acp.NewAvailableCommand("title", "Set or show the session title", "[new title]"),
		Execute: func(ctx context.Context, args string) ([]acp.ContentBlock, error) {
			sess := core.SessionFrom(ctx)
			if args != "" {
				sess.Title = args
				store.Set(ctx, sess.SessionID, sess)
			}
			agentutil.NewSessionStream(core.ClientFrom(ctx), sess.SessionID).SendSessionInfo(ctx, sess.Title)
			return nil, nil
		},
	}
}

// UsageCommand shows current token usage and cost.
func UsageCommand() Command {
	return Command{
		AvailableCommand: acp.NewAvailableCommand("usage", "Show token usage and cost", ""),
		Execute: func(ctx context.Context, _ string) ([]acp.ContentBlock, error) {
			sess := core.SessionFrom(ctx)
			agentutil.NewSessionStream(core.ClientFrom(ctx), sess.SessionID).SendUsageUpdate(ctx, sess.Usage.TotalTokens, 0, sess.Cost)
			return nil, nil
		},
	}
}

// CommandsCommand lists available slash commands.
func CommandsCommand(getCommands func() []acp.AvailableCommand) Command {
	return Command{
		AvailableCommand: acp.NewAvailableCommand("commands", "List available slash commands", ""),
		Execute: func(ctx context.Context, _ string) ([]acp.ContentBlock, error) {
			sess := core.SessionFrom(ctx)
			agentutil.NewSessionStream(core.ClientFrom(ctx), sess.SessionID).SendAvailableCommands(ctx, getCommands()...)
			return nil, nil
		},
	}
}

// InfoCommand shows session details.
func InfoCommand() Command {
	return Command{
		AvailableCommand: acp.NewAvailableCommand("info", "Show session details", ""),
		Execute: func(ctx context.Context, _ string) ([]acp.ContentBlock, error) {
			sess := core.SessionFrom(ctx)
			agentutil.NewSessionStream(core.ClientFrom(ctx), sess.SessionID).SendSessionInfo(ctx, sess.Title)
			return nil, nil
		},
	}
}
