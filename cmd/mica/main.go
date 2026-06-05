package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"charm.land/catwalk/pkg/embedded"
	acp "github.com/carsonfarmer/go-acp-sdk"
	"github.com/carsonfarmer/go-acp-sdk/middleware"
	"github.com/carsonfarmer/go-acp-sdk/storage"
	"github.com/carsonfarmer/go-acp-sdk/ws"
	"github.com/carsonfarmer/mica/pkg/agent"
	"github.com/carsonfarmer/mica/pkg/llm"
)

var (
	dataDir = flag.String("data", ".mica", "data directory for persistence")
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

func run(ctx context.Context) error {
	all := embedded.GetAll()

	reg := llm.NewRegistry()
	for _, cw := range all {
		if err := reg.AddProvider(cw); err != nil {
			log.Printf("skipping provider %s: %v", cw.ID, err)
		}
	}
	def := reg.Default()
	if def.Provider == "" {
		return fmt.Errorf("no providers configured; set at least one API key environment variable")
	}
	log.Printf("loaded %d provider(s), default model %s", len(reg.Providers()), def)

	store, err := storage.NewTypedDirStore[*agent.AgentSession](*dataDir)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}
	defer store.Close()
	log.Printf("persisting sessions to %s", *dataDir)

	ag := agent.New(reg, store,
		agent.WithTools(
			llm.ReadFileTool(),
			llm.WriteFileTool(),
			llm.TerminalTool(),
			llm.PlanTool(),
			llm.EditTool(),
		),
	)

	endpoint := fmt.Sprintf("ws://localhost%s/acp", *addr)
	log.Printf("stdio proxy relaying to %s", endpoint)
	go func() { ws.Proxy(ctx, endpoint) }()

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
