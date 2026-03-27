package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/frontend"
	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/mock"
	"github.com/agent-racer/backend/internal/monitor"
	"github.com/agent-racer/backend/internal/replay"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/tracks"
	"github.com/agent-racer/backend/internal/ws"
)

var version = "dev"

type serverOptions struct {
	mockMode    bool
	devMode     bool
	configPath  string
	port        int
	showVersion bool
}

func buildSources(cfg *config.Config) []monitor.Source {
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
	return sources
}

func parseArgs(args []string, output io.Writer) (serverOptions, error) {
	var opts serverOptions

	fs := flag.NewFlagSet("agent-racer-server", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.BoolVar(&opts.mockMode, "mock", false, "Use mock session data")
	fs.BoolVar(&opts.devMode, "dev", false, "Development mode (serve frontend from filesystem)")
	fs.StringVar(&opts.configPath, "config", "", "Path to config file (defaults to ~/.config/agent-racer/config.yaml)")
	fs.IntVar(&opts.port, "port", 0, "Override server port")
	fs.BoolVar(&opts.showVersion, "version", false, "Print version information and exit")

	if err := fs.Parse(args); err != nil {
		return serverOptions{}, err
	}

	return opts, nil
}

func printVersion(output io.Writer) {
	_, _ = fmt.Fprintln(output, version)
}

func main() {
	opts, err := parseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	if opts.showVersion {
		printVersion(os.Stdout)
		return
	}

	// Use XDG config directory if no config path specified
	cfgPath := opts.configPath
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}

	cfg, cfgWarnings, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	for _, w := range cfgWarnings {
		log.Printf("Config warning: %s", w)
	}

	if opts.port > 0 {
		cfg.Server.Port = opts.port
	}

	store := session.NewStore()
	broadcaster := ws.NewBroadcaster(store, cfg.Monitor.BroadcastThrottle, cfg.Monitor.SnapshotInterval, cfg.Server.MaxConnections)
	broadcaster.SetPrivacyFilter(cfg.Privacy.NewPrivacyFilter())

	frontendDir := ""
	if opts.devMode {
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
	if !opts.devMode {
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

	authToken := config.NormalizeAuthToken(cfg.Server.AuthToken)
	if config.IsWeakAuthToken(authToken) {
		log.Println("========================================")
		log.Printf("  WARNING: Weak auth_token %q is not allowed.", authToken)
		log.Println("  Generating a random token for this startup.")
		log.Println("  Use a long random value in server.auth_token to persist.")
		log.Println("========================================")
		authToken = ""
	}
	if authToken == "" {
		var err error
		authToken, err = config.GenerateToken()
		if err != nil {
			log.Fatalf("Failed to generate auth token: %v", err)
		}
		log.Println("========================================")
		log.Println("  WARNING: No auth_token configured.")
		log.Printf("  Generated token: %s", authToken)
		log.Printf("  Open: http://%s:%d/#token=%s", cfg.Server.Host, cfg.Server.Port, authToken)
		log.Println("  The token is read from URL fragment and then removed from the address bar.")
		log.Println("  Set server.auth_token in config to persist.")
		log.Println("========================================")
	}

	// Set up replay recorder and API (records session snapshots to JSONL files).
	replayDir := config.DefaultReplayDir()
	var rec *replay.Recorder
	if cfg.Replay.Enabled {
		var recErr error
		rec, recErr = replay.NewRecorder(replayDir, cfg.Replay.RetentionDays)
		if recErr != nil {
			log.Printf("Replay recorder disabled: %v", recErr)
		}
	}

	server := ws.NewServer(cfg, store, broadcaster, frontendDir, opts.devMode, embeddedHandler, cfg.Server.AllowedOrigins, authToken)

	// Track store for custom race circuits.
	trackStore, trackErr := tracks.NewStore("")
	if trackErr != nil {
		log.Printf("Warning: track store unavailable: %v", trackErr)
	} else {
		server.SetTrackHandler(tracks.NewHandler(trackStore))
	}

	// Stats tracker for gamification system.
	gamStore := gamification.NewStore("")
	seasonCfg := &gamification.SeasonConfig{
		Enabled: cfg.Gamification.BattlePass.Enabled,
		Season:  cfg.Gamification.BattlePass.Season,
	}
	tracker, statsCh, err := gamification.NewStatsTracker(gamStore, cfg.Monitor.StatsEventBuffer, seasonCfg)
	if err != nil {
		log.Fatalf("Failed to initialize stats tracker: %v", err)
	}

	tracker.OnBattlePassProgress(func(progress gamification.BattlePassProgress, recentXP []gamification.XPEntry) {
		broadcaster.BroadcastBattlePassProgress(ws.BattlePassProgressPayload{
			XP:           progress.XP,
			Tier:         progress.Tier,
			TierProgress: progress.Pct,
			RecentXP:     recentXP,
			Rewards:      progress.Rewards,
		})
	})

	tracker.OnAchievement(func(a gamification.Achievement, rw *gamification.Reward) {
		payload := ws.AchievementUnlockedPayload{
			ID:          a.ID,
			Name:        a.Name,
			Description: a.Description,
			Tier:        string(a.Tier),
		}
		if rw != nil {
			payload.Reward = &ws.AchievementRewardPayload{
				Type: string(rw.Type),
				ID:   rw.ID,
				Name: rw.Name,
			}
		}
		broadcaster.BroadcastAchievement(payload)
	})

	server.SetStatsTracker(tracker)

	// Wire up replay API handler (serves replays even when recording is disabled).
	replayAPIHandler := replay.NewHandler(replayDir, server.Authorize)
	server.SetReplayHandler(replayAPIHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tracker.Run(ctx)
	}()

	var mon *monitor.Monitor
	if opts.mockMode {
		log.Println("Starting in mock mode")
		gen := mock.NewGenerator(store, broadcaster, cfg.Monitor.MockTickInterval)
		gen.SetStatsEvents(statsCh)
		gen.Start(ctx)
	} else {
		log.Println("Starting in real mode (process monitoring)")
		sources := buildSources(cfg)
		mon = monitor.NewMonitor(cfg, store, broadcaster, sources)
		mon.SetStatsEvents(statsCh)
		if rec != nil {
			mon.SetSnapshotHook(rec.WriteSnapshot)
		}
		go mon.Start(ctx)
	}

	mux := http.NewServeMux()
	server.SetupRoutes(mux)
	httpServer := ws.NewHTTPServer(cfg.Server.Host, cfg.Server.Port, mux)

	// SIGHUP: reload config.yaml and apply changes at runtime.
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	defer signal.Stop(sighupCh)
	go func() {
		for range sighupCh {
			newCfg, reloadWarnings, err := config.LoadOrDefault(cfgPath)
			if err != nil {
				log.Printf("Config reload failed: %v", err)
				continue
			}
			for _, w := range reloadWarnings {
				log.Printf("Config warning: %s", w)
			}

			changes := config.Diff(cfg, newCfg)
			if len(changes) == 0 {
				log.Println("Config reloaded: no changes detected")
				continue
			}

			for _, c := range changes {
				log.Printf("Config changed: %s", c)
			}

			// Apply privacy filter (always safe to update).
			broadcaster.SetPrivacyFilter(newCfg.Privacy.NewPrivacyFilter())

			// Apply broadcaster timing changes.
			if cfg.Monitor.BroadcastThrottle != newCfg.Monitor.BroadcastThrottle ||
				cfg.Monitor.SnapshotInterval != newCfg.Monitor.SnapshotInterval {
				broadcaster.SetConfig(newCfg.Monitor.BroadcastThrottle, newCfg.Monitor.SnapshotInterval)
			}

			// Apply monitor-level config (models, token norm, timings).
			if mon != nil {
				mon.SetConfig(newCfg)

				// Rebuild sources if source configuration changed.
				if cfg.Sources != newCfg.Sources {
					mon.SetSources(buildSources(newCfg))
				}
			}

			cfg = newCfg
			log.Printf("Config reload complete (%d change(s) applied)", len(changes))
		}
	}()

	cleanup := func() {
		cancel()
		broadcaster.Stop()
		wg.Wait() // allow stats tracker to flush
		if rec != nil {
			rec.Close()
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		sig := <-sigCh
		log.Printf("Shutting down after signal: %s", sig)
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP shutdown error: %v", err)
		}
	}()

	log.Printf("Server listening on %s", httpServer.Addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		cleanup()
		log.Fatalf("Server error: %v", err)
	}
	cleanup()
}
