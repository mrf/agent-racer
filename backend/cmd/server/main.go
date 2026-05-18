package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	awmonitor "github.com/mrf/agentwatch/monitor"
	awsource "github.com/mrf/agentwatch/source"
	awclaude "github.com/mrf/agentwatch/sources/claude"
	awcodex "github.com/mrf/agentwatch/sources/codex"
	awgemini "github.com/mrf/agentwatch/sources/gemini"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/frontend"
	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/mock"
	"github.com/agent-racer/backend/internal/monitor"
	"github.com/agent-racer/backend/internal/racer"
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

// buildRegistry creates an agentwatch source.Registry populated according
// to the enabled sources in cfg.
func buildRegistry(cfg *config.Config) *awsource.Registry {
	reg := awsource.NewRegistry()
	home, _ := os.UserHomeDir()

	if cfg.Sources.Claude {
		_ = awclaude.Register(reg,
			awclaude.WithRoot(filepath.Join(home, ".claude", "projects")),
			awclaude.WithSessionEndDir(cfg.Monitor.SessionEndDir),
		)
	}
	if cfg.Sources.Codex {
		codexRoot := os.Getenv("CODEX_HOME")
		if codexRoot == "" {
			codexRoot = filepath.Join(home, ".codex")
		}
		_ = awcodex.Register(reg,
			awcodex.WithRoot(codexRoot),
			awcodex.WithDiscoverWindow(10*time.Minute),
		)
	}
	if cfg.Sources.Gemini {
		_ = awgemini.Register(reg,
			awgemini.WithRoot(filepath.Join(home, ".gemini", "tmp")),
		)
	}

	return reg
}

// registrySources instantiates all sources registered in the given Registry.
func registrySources(reg *awsource.Registry) []awsource.Source {
	names := reg.Names()
	sources := make([]awsource.Source, 0, len(names))
	for i := 0; i < len(names); i++ {
		f, ok := reg.Get(names[i])
		if !ok {
			continue
		}
		src, err := f()
		if err != nil {
			slog.Error("failed to build source", "component", "server", "source", names[i], "error", err)
			continue
		}
		sources = append(sources, src)
	}
	return sources
}

// buildSources is a convenience wrapper used by tests.
func buildSources(cfg *config.Config) []awsource.Source {
	return registrySources(buildRegistry(cfg))
}

// buildAWMonitor creates an agentwatch monitor.Monitor configured from cfg.
// Returns nil if sources is empty (agentwatch requires at least one).
func buildAWMonitor(cfg *config.Config, sources []awsource.Source, sink awmonitor.EventSink) *awmonitor.Monitor {
	if len(sources) == 0 {
		return nil
	}
	threshold := cfg.Monitor.HealthWarningThreshold
	if threshold <= 0 {
		threshold = 3
	}
	awMon, err := awmonitor.New(
		awmonitor.WithSources(sources...),
		awmonitor.WithPollInterval(cfg.Monitor.PollInterval),
		awmonitor.WithSink(sink),
		awmonitor.WithStaleThreshold(cfg.Monitor.SessionStaleAfter),
		awmonitor.WithCompletionRetention(cfg.Monitor.CompletionRemoveAfter),
		awmonitor.WithHealthThreshold(threshold),
	)
	if err != nil {
		slog.Error("failed to create agentwatch monitor", "component", "server", "error", err)
		return nil
	}
	return awMon
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

	// Set up structured JSON logging.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	// Use XDG config directory if no config path specified
	cfgPath := opts.configPath
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}

	cfg, cfgWarnings, err := config.LoadOrDefault(cfgPath)
	if err != nil {
		slog.Error("failed to load config", "component", "server", "error", err)
		os.Exit(1)
	}
	for _, w := range cfgWarnings {
		slog.Warn("config warning", "component", "server", "message", w)
	}

	if opts.port > 0 {
		cfg.Server.Port = opts.port
	}

	// Validate TLS config: both cert and key must be provided together.
	if (cfg.Server.TLSCert == "") != (cfg.Server.TLSKey == "") {
		slog.Error("TLS misconfigured: both tls_cert and tls_key must be set (or both empty)", "component", "server")
		os.Exit(1)
	}
	if cfg.Server.TLSEnabled() {
		slog.Info("TLS enabled", "component", "server", "cert", cfg.Server.TLSCert, "key", cfg.Server.TLSKey)
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

	// Verify embedded frontend integrity before serving.
	if err := frontend.Verify(); err != nil {
		slog.Error("frontend integrity check failed", "component", "server", "error", err)
		os.Exit(1)
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
				slog.Info("no embedded frontend, falling back to filesystem", "component", "server", "dir", fallback)
				embeddedHandler = http.FileServer(http.Dir(fallback))
			}
		}
	}

	authToken := config.NormalizeAuthToken(cfg.Server.AuthToken)
	if config.IsWeakAuthToken(authToken) {
		slog.Warn("weak auth token rejected, generating random token",
			"component", "server", "rejected_token", authToken)
		authToken = ""
	}
	if authToken == "" {
		var err error
		authToken, err = config.GenerateToken()
		if err != nil {
			slog.Error("failed to generate auth token", "component", "server", "error", err)
			os.Exit(1)
		}
		slog.Warn("no auth token configured, generated random token",
			"component", "server",
			"token", authToken,
			"url", fmt.Sprintf("%s://%s:%d/#token=%s", cfg.Server.Scheme(), cfg.Server.Host, cfg.Server.Port, authToken),
		)
	}

	// Set up replay recorder and API (records session snapshots to JSONL files).
	replayDir := config.DefaultReplayDir()
	var rec *replay.Recorder
	if cfg.Replay.Enabled {
		var recErr error
		rec, recErr = replay.NewRecorder(replayDir, cfg.Replay.RetentionDays)
		if recErr != nil {
			slog.Warn("replay recorder disabled", "component", "server", "error", recErr)
		}
		if rec != nil {
			rec.SetPrivacyFilter(cfg.Privacy.NewPrivacyFilter())
		}
	}

	server := ws.NewServer(cfg, store, broadcaster, frontendDir, opts.devMode, embeddedHandler, cfg.Server.AllowedOrigins, authToken)

	// cfgMu serializes read-modify-write cycles on the server config
	// (SIGHUP reload and track-handler updates).
	var cfgMu sync.Mutex

	// Track store for custom race circuits.
	trackStore, trackErr := tracks.NewStore("")
	if trackErr != nil {
		slog.Warn("track store unavailable", "component", "server", "error", trackErr)
	} else {
		th := tracks.NewHandler(trackStore)
		th.SetActiveTrackProvider(
			func() string { return server.Config().Track.Active },
			func(id string) error {
				cfgMu.Lock()
				old := server.Config()
				newCfg := *old
				newCfg.Track.Active = id
				server.SetConfig(&newCfg)
				cfgMu.Unlock()
				broadcaster.BroadcastSnapshot()
				return nil
			},
		)
		server.SetTrackHandler(th)
		broadcaster.SetActiveTrackProvider(func() string {
			return server.Config().Track.Active
		})
	}

	// Stats tracker for gamification system.
	gamStore := gamification.NewStore("")
	seasonCfg := &gamification.SeasonConfig{
		Enabled: cfg.Gamification.BattlePass.Enabled,
		Season:  cfg.Gamification.BattlePass.Season,
	}
	tracker, statsCh, err := gamification.NewStatsTracker(gamStore, cfg.Monitor.StatsEventBuffer, seasonCfg)
	if err != nil {
		slog.Error("failed to initialize stats tracker", "component", "server", "error", err)
		os.Exit(1)
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
	var monitorSink awmonitor.EventSink // reused across SIGHUP rebuilds
	if opts.mockMode {
		slog.Info("starting in mock mode", "component", "server")
		gen := mock.NewGenerator(store, broadcaster, cfg.Monitor.MockTickInterval)
		gen.SetStatsEvents(statsCh)
		gen.Start(ctx)
	} else {
		slog.Info("starting in real mode", "component", "server")

		// Build sources via agentwatch Registry.
		reg := buildRegistry(cfg)
		sources := registrySources(reg)

		// Internal monitor handles enrichment (process activity, tmux,
		// token normalization, burn rate) and feeds the session store +
		// broadcaster pipeline.
		mon = monitor.NewMonitor(cfg, store, broadcaster, nil)
		mon.SetStatsEvents(statsCh)
		if rec != nil {
			mon.SetSnapshotHook(rec.WriteSnapshot)
		}

		// Wire racer.Sink (position/lane/overtake tracking) alongside the
		// internal monitor as a multi-sink for the agentwatch monitor.
		racerSink := racer.NewSink()
		monitorSink = awmonitor.NewMultiSink(
			awmonitor.EventSinkFunc(mon.HandleEvent),
			racerSink,
		)

		awMon := buildAWMonitor(cfg, sources, monitorSink)
		mon.SetAWMonitor(awMon)

		server.SetHealthCheck(mon.SourceHealthSnapshot)
		server.SetHealthHook(mon.SourceHealthSnapshot)
		go mon.Start(ctx)
	}

	mux := http.NewServeMux()
	server.SetupRoutes(mux)
	httpServer := ws.NewHTTPServer(cfg.Server.Host, cfg.Server.Port, cfg.Server.TLSEnabled(), mux)

	// SIGHUP: reload config.yaml and apply changes at runtime.
	sighupCh := make(chan os.Signal, 1)
	signal.Notify(sighupCh, syscall.SIGHUP)
	sighupDone := make(chan struct{})
	go func() {
		defer close(sighupDone)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sighupCh:
			}

			newCfg, reloadWarnings, err := config.LoadOrDefault(cfgPath)
			if err != nil {
				slog.Error("config reload failed", "component", "server", "error", err)
				continue
			}
			for _, w := range reloadWarnings {
				slog.Warn("config warning", "component", "server", "message", w)
			}

			changes := func() []string {
				cfgMu.Lock()
				defer cfgMu.Unlock()

				oldCfg := server.Config()
				changes := config.Diff(oldCfg, newCfg)
				if len(changes) == 0 {
					return nil
				}

				// Apply privacy filter (always safe to update).
				pf := newCfg.Privacy.NewPrivacyFilter()
				broadcaster.SetPrivacyFilter(pf)
				if rec != nil {
					rec.SetPrivacyFilter(pf)
				}

				// Apply broadcaster timing changes.
				if oldCfg.Monitor.BroadcastThrottle != newCfg.Monitor.BroadcastThrottle ||
					oldCfg.Monitor.SnapshotInterval != newCfg.Monitor.SnapshotInterval {
					broadcaster.SetConfig(newCfg.Monitor.BroadcastThrottle, newCfg.Monitor.SnapshotInterval)
				}

				// Apply monitor-level config (models, token norm, timings).
				if mon != nil {
					// Rebuild the agentwatch monitor when source config or
					// immutable timing settings change.
					needsRebuild := oldCfg.Sources != newCfg.Sources ||
						oldCfg.Monitor.SessionStaleAfter != newCfg.Monitor.SessionStaleAfter ||
						oldCfg.Monitor.CompletionRemoveAfter != newCfg.Monitor.CompletionRemoveAfter ||
						oldCfg.Monitor.HealthWarningThreshold != newCfg.Monitor.HealthWarningThreshold
					if needsRebuild {
						newSources := registrySources(buildRegistry(newCfg))
						newAwMon := buildAWMonitor(newCfg, newSources, monitorSink)
						mon.SetAWMonitor(newAwMon)
					}

					mon.SetConfig(newCfg)
				}

				server.SetConfig(newCfg)
				return changes
			}()

			if len(changes) == 0 {
				slog.Info("config reloaded, no changes detected", "component", "server")
				continue
			}
			for _, c := range changes {
				slog.Info("config changed", "component", "server", "change", c)
			}
		}
	}()

	cleanup := func() {
		signal.Stop(sighupCh)
		cancel()
		<-sighupDone // wait for any in-flight reload to finish
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
		slog.Info("shutting down", "component", "server", "signal", sig)
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP shutdown error", "component", "server", "error", err)
		}
	}()

	slog.Info("server listening", "component", "server", "addr", httpServer.Addr, "scheme", cfg.Server.Scheme())
	var listenErr error
	if cfg.Server.TLSEnabled() {
		listenErr = httpServer.ListenAndServeTLS(cfg.Server.TLSCert, cfg.Server.TLSKey)
	} else {
		listenErr = httpServer.ListenAndServe()
	}
	if listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
		cleanup()
		slog.Error("server error", "component", "server", "error", listenErr)
		os.Exit(1)
	}
	cleanup()
}
