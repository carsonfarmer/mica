package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"charm.land/catwalk/pkg/embedded"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/middleware"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/go-acp-sdk/ws"
	"github.com/carsonfarmer/mica/pkg/agent"
	"github.com/carsonfarmer/mica/pkg/core"
	"github.com/carsonfarmer/mica/pkg/tools"
)

var (
	dataDir = flag.String("data", "", "data directory for persistence (default ~/.mica)")
	addr    = flag.String("http", ":8080", "HTTP address to listen on")
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "mica: %v\n", err)
		os.Exit(1)
	}
}

func isRunning(addr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost"+addr+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func run(ctx context.Context) error {
	wsEndpoint := fmt.Sprintf("ws://localhost%s/acp", *addr)

	// Check if an existing instance is already running. If so, become a
	// client instead of starting a new server.
	if isRunning(*addr) {
		log.Printf("found existing instance at localhost%s, running as client", *addr)
		ws.Proxy(ctx, wsEndpoint)
		return nil
	}

	// Resolve data dir to absolute so sessions are findable regardless of CWD.
	d := *dataDir
	if d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".mica")
	}
	absData, err := filepath.Abs(d)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}

	all := embedded.GetAll()

	reg := core.NewRegistry()
	for _, cw := range all {
		if err := reg.AddProvider(cw); err != nil {
			log.Printf("skipping provider %s: %v", cw.ID, err)
		}
	}
	def := reg.Default()
	if def == "" {
		return fmt.Errorf("no providers configured; set at least one API key environment variable")
	}
	log.Printf("loaded %d provider(s), default model %s", len(reg.Providers()), def)

	store, err := storage.NewTypedDirStore[*core.AgentSession](absData)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer store.Close()
	log.Printf("persisting sessions to %s", absData)

	ag := agent.New(reg, store,
		agent.WithCommands(
			agent.CompactCommand(store, reg),
			agent.TitleCommand(store),
			agent.InfoCommand(),
			agent.UsageCommand(),
		),
		agent.WithTools(
			tools.ReadTool(),
			tools.WriteTool(),
			tools.TerminalTool(),
			tools.PlanTool(),
			tools.EditTool(),
			tools.CompactTool(store, reg),
		),
	)
	agent.WithCommands(agent.CommandsCommand(ag.GetAvailableCommands))(ag)

	log.Printf("stdio proxy relaying to %s", wsEndpoint)
	go func() { ws.Proxy(ctx, wsEndpoint) }()

	mux := http.NewServeMux()
	mux.Handle("/acp", ws.NewHandler(ag,
		ws.WithCheckOrigin(ws.AllowAllOrigins),
		ws.WithConnOptions(acp.WithMiddleware(middleware.Logging(nil))),
	))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	srv := &http.Server{Addr: *addr, Handler: mux}

	go func() {
		log.Printf("listening on http://localhost%s/acp", *addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	log.Printf("mica agent started (HTTP %s)", *addr)
	<-ctx.Done()
	srv.Shutdown(context.Background())
	return nil
}
