package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/frontend"
	"github.com/agent-racer/backend/internal/mock"
	"github.com/agent-racer/backend/internal/monitor"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/ws"
)

func main() {
	mockMode := flag.Bool("mock", false, "Use mock session data")
	devMode := flag.Bool("dev", false, "Development mode (serve frontend from filesystem)")
	configPath := flag.String("config", "", "Path to config file (defaults to ~/.config/agent-racer/config.yaml)")
	port := flag.Int("port", 0, "Override server port")
	flag.Parse()

	// Use XDG config directory if no config path specified
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}

	cfg, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *port > 0 {
		cfg.Server.Port = *port
	}

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, cfg.Monitor.BroadcastThrottle, cfg.Monitor.SnapshotInterval)
	broadcaster.SetPrivacyFilter(cfg.Privacy.NewPrivacyFilter())

	frontendDir := ""
	if *devMode {
		exe, _ := os.Executable()
		frontendDir = filepath.Join(filepath.Dir(exe), "..", "..", "frontend")
		// If running with go run, the exe path is in a temp dir, use CWD instead
		if _, err := os.Stat(frontendDir); os.IsNotExist(err) {
			cwd, _ := os.Getwd()
			frontendDir = filepath.Join(cwd, "..", "frontend")
		}
	}

	// Embedded frontend handler: when built with -tags embed, serves from binary.
	// Otherwise falls back to serving from the filesystem.
	var embeddedHandler http.Handler
	if !*devMode {
		embeddedHandler = frontend.Handler()
		if embeddedHandler == nil {
			cwd, _ := os.Getwd()
			fallback := filepath.Join(cwd, "..", "frontend")
			if _, err := os.Stat(fallback); err == nil {
				log.Printf("No embedded frontend, falling back to: %s", fallback)
				embeddedHandler = http.FileServer(http.Dir(fallback))
			}
		}
	}

	server := ws.NewServer(cfg, store, broadcaster, frontendDir, *devMode, embeddedHandler, cfg.Server.AllowedOrigins, cfg.Server.AuthToken)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *mockMode {
		log.Println("Starting in mock mode")
		gen := mock.NewGenerator(store, broadcaster)
		gen.Start(ctx)
	} else {
		log.Println("Starting in real mode (process monitoring)")
		var sources []monitor.Source
		if cfg.Sources.Claude {
			sources = append(sources, monitor.NewClaudeSource(10*time.Minute))
		}
		if cfg.Sources.Codex {
			sources = append(sources, monitor.NewCodexSource(10*time.Minute))
		}
		if cfg.Sources.Gemini {
			sources = append(sources, monitor.NewGeminiSource(10*time.Minute))
		}
		mon := monitor.NewMonitor(cfg, store, broadcaster, sources)
		go mon.Start(ctx)
	}

	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		cancel()
		os.Exit(0)
	}()

	if err := ws.ListenAndServe(cfg.Server.Host, cfg.Server.Port, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
