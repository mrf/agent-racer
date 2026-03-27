package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/replay"
	"github.com/agent-racer/backend/internal/session"
	"github.com/agent-racer/backend/internal/tracks"
	"github.com/gorilla/websocket"
)

// validTmuxTarget matches a tmux target like "session:window.pane" where the
// session name contains only safe characters and window/pane are integers.
var validTmuxTarget = regexp.MustCompile(`^[a-zA-Z0-9_.-]+:\d+\.\d+$`)

const (
	// maxRequestBodySize is the maximum allowed size for JSON request bodies (1 MB).
	maxRequestBodySize int64 = 1 << 20
	// maxWSAuthMessageSize is the maximum allowed size for the WebSocket auth message (4 KB).
	maxWSAuthMessageSize int64 = 4 << 10
)

// tmuxFocusSession switches to the tmux pane identified by target (e.g. "main:2.0").
func tmuxFocusSession(target string) error {
	if !validTmuxTarget.MatchString(target) {
		return fmt.Errorf("invalid tmux target %q", target)
	}
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	if err := exec.Command(tmuxPath, "select-window", "-t", target).Run(); err != nil {
		return fmt.Errorf("select-window: %w", err)
	}
	if err := exec.Command(tmuxPath, "select-pane", "-t", target).Run(); err != nil {
		return fmt.Errorf("select-pane: %w", err)
	}
	return nil
}

type Server struct {
	config            atomic.Pointer[config.Config]
	store             *session.Store
	broadcaster       *Broadcaster
	frontendDir       string
	dev               bool
	embeddedHandler   http.Handler
	allowedOrigins    map[string]bool
	allowedHosts      map[string]bool
	authToken         string
	tracker           *gamification.StatsTracker
	achievementEngine *gamification.AchievementEngine
	rewardRegistry    *gamification.RewardRegistry
	replayHandler     *replay.Handler
	trackHandler      *tracks.Handler
	apiRateLimiter    *clientRateLimiter
	wsAuthRateLimiter *clientRateLimiter
	healthHook        func() []SourceHealthPayload
	startTime         time.Time
}

func NewServer(cfg *config.Config, store *session.Store, broadcaster *Broadcaster, frontendDir string, dev bool, embeddedHandler http.Handler, allowedOrigins []string, authToken string) *Server {
	s := &Server{
		store:             store,
		broadcaster:       broadcaster,
		frontendDir:       frontendDir,
		dev:               dev,
		embeddedHandler:   embeddedHandler,
		allowedOrigins:    make(map[string]bool),
		allowedHosts:      make(map[string]bool),
		authToken:         authToken,
		achievementEngine: gamification.NewAchievementEngine(),
		rewardRegistry:    gamification.NewRewardRegistry(),
		apiRateLimiter:    newClientRateLimiter(120, time.Minute, 30),
		wsAuthRateLimiter: newClientRateLimiter(12, time.Minute, 4),
		startTime:         time.Now(),
	}
	s.config.Store(cfg)

	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		s.allowedOrigins[trimmed] = true
		if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
			s.allowedHosts[parsed.Host] = true
		}
	}

	return s
}

// Config returns the current configuration (safe for concurrent use).
func (s *Server) Config() *config.Config {
	return s.config.Load()
}

// SetConfig atomically replaces the server's configuration.
func (s *Server) SetConfig(cfg *config.Config) {
	s.config.Store(cfg)
}

// SetStatsTracker configures the stats tracker used by the /api/stats endpoint.
// Must be called before SetupRoutes.
func (s *Server) SetStatsTracker(tracker *gamification.StatsTracker) {
	s.tracker = tracker
}

// SetReplayHandler configures the replay API handler. Must be called before SetupRoutes.
func (s *Server) SetReplayHandler(h *replay.Handler) {
	s.replayHandler = h
}

// SetTrackHandler configures the track handler used by /api/tracks endpoints.
// Must be called before SetupRoutes.
func (s *Server) SetTrackHandler(h *tracks.Handler) {
	s.trackHandler = h
}

// SetHealthHook registers a function that returns source health status.
// Used by the /healthz endpoint. Must be called before SetupRoutes.
func (s *Server) SetHealthHook(hook func() []SourceHealthPayload) {
	s.healthHook = hook
}

// healthzResponse is the JSON shape returned by GET /healthz.
type healthzResponse struct {
	Status        string                `json:"status"`
	Uptime        string                `json:"uptime"`
	UptimeSeconds float64               `json:"uptimeSeconds"`
	Sources       []SourceHealthPayload `json:"sources,omitempty"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(s.startTime)
	resp := healthzResponse{
		Status:        "ok",
		Uptime:        uptime.Truncate(time.Second).String(),
		UptimeSeconds: uptime.Seconds(),
	}

	if s.healthHook != nil {
		resp.Sources = s.healthHook()
		for _, src := range resp.Sources {
			if src.Status == StatusFailed || src.Status == StatusDegraded {
				resp.Status = "degraded"
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) SetupRoutes(mux *http.ServeMux) {
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/sessions", s.handleSessions)
	apiMux.HandleFunc("/api/sessions/", s.handleSessionRoutes)
	apiMux.HandleFunc("/api/config", s.handleConfig)
	apiMux.HandleFunc("/api/stats", s.handleStats)
	apiMux.HandleFunc("/api/achievements", s.handleAchievements)
	apiMux.HandleFunc("/api/equip", s.handleEquip)
	apiMux.HandleFunc("/api/unequip", s.handleUnequip)
	apiMux.HandleFunc("/api/challenges", s.handleChallenges)

	if s.replayHandler != nil {
		s.replayHandler.RegisterRoutes(apiMux)
	}

	if s.trackHandler != nil {
		tracksAuth := func(w http.ResponseWriter, r *http.Request) {
			if !s.authorize(r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			s.trackHandler.ServeHTTP(w, r)
		}
		apiMux.HandleFunc("/api/tracks", tracksAuth)
		apiMux.HandleFunc("/api/tracks/", tracksAuth)
	}

	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.Handle("/ws", s.rateLimitWS(http.HandlerFunc(s.handleWS)))
	mux.Handle("/api/", s.rateLimitAPI(apiMux))

	if s.dev {
		slog.Info("serving frontend from filesystem", "dir", s.frontendDir)
		mux.Handle("/", http.FileServer(http.Dir(s.frontendDir)))
	} else if s.embeddedHandler != nil {
		slog.Info("serving embedded frontend")
		mux.Handle("/", s.embeddedHandler)
	}
}

// wsAuthMessage is the first message a WebSocket client sends to authenticate.
type wsAuthMessage struct {
	Type  string `json:"type"`
	Token string `json:"token"`
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: s.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "error", err)
		return
	}

	if s.authToken != "" {
		conn.SetReadLimit(maxWSAuthMessageSize)
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		_ = conn.SetReadDeadline(time.Time{})
		if err != nil {
			_ = conn.Close()
			return
		}
		var auth wsAuthMessage
		if err := json.Unmarshal(msg, &auth); err != nil || auth.Type != "auth" || auth.Token != s.authToken {
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "unauthorized"))
			_ = conn.Close()
			return
		}
	}

	c, err := s.broadcaster.AddClient(conn)
	if err != nil {
		slog.Warn("websocket rejected", "addr", r.RemoteAddr, "error", err)
		return
	}
	slog.Info("websocket client connected", "addr", r.RemoteAddr)

	go func() {
		defer func() {
			s.broadcaster.RemoveClient(c)
			slog.Info("websocket client disconnected", "addr", r.RemoteAddr)
		}()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(msg, &req) == nil && req.Type == "resync" {
				s.broadcaster.SendSnapshot(c)
			}
		}
	}()
}

func (s *Server) rateLimitAPI(next http.Handler) http.Handler {
	return s.rateLimitHTTP(next, s.apiRateLimiter)
}

func (s *Server) rateLimitWS(next http.Handler) http.Handler {
	return s.rateLimitHTTP(next, s.wsAuthRateLimiter)
}

func (s *Server) rateLimitHTTP(next http.Handler, limiter *clientRateLimiter) http.Handler {
	if limiter == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decision := limiter.Allow(clientAddress(r))
		if !decision.Allowed {
			writeRateLimitExceeded(w, decision.RetryAfter)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	sessions := s.broadcaster.FilterSessions(s.store.GetAll())
	_ = json.NewEncoder(w).Encode(sessions)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.Config().Sound)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if s.tracker == nil {
		http.Error(w, "stats not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.tracker.Stats())
}

// achievementResponse is the JSON shape returned by /api/achievements.
type achievementResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Tier        string     `json:"tier"`
	Category    string     `json:"category"`
	Unlocked    bool       `json:"unlocked"`
	UnlockedAt  *time.Time `json:"unlockedAt,omitempty"`
}

func (s *Server) handleAchievements(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	registry := s.achievementEngine.Registry()

	var unlocked map[string]time.Time
	if s.tracker != nil {
		unlocked = s.tracker.Stats().AchievementsUnlocked
	}

	out := make([]achievementResponse, 0, len(registry))
	for _, a := range registry {
		resp := achievementResponse{
			ID:          a.ID,
			Name:        a.Name,
			Description: a.Description,
			Tier:        string(a.Tier),
			Category:    string(a.Category),
		}
		if t, ok := unlocked[a.ID]; ok {
			resp.Unlocked = true
			resp.UnlockedAt = &t
		}
		out = append(out, resp)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) handleChallenges(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.tracker == nil {
		http.Error(w, "stats not available", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.tracker.Challenges())
}

type equipRequest struct {
	RewardID string `json:"rewardId"`
	Slot     string `json:"slot"`
}

func (s *Server) handleEquip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.tracker == nil {
		http.Error(w, "stats not available", http.StatusServiceUnavailable)
		return
	}

	var req equipRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.RewardID == "" {
		http.Error(w, "rewardId is required", http.StatusBadRequest)
		return
	}
	if req.Slot == "" {
		http.Error(w, "slot is required", http.StatusBadRequest)
		return
	}

	slot := gamification.RewardType(req.Slot)
	if !gamification.ValidSlot(slot) {
		http.Error(w, "invalid slot", http.StatusBadRequest)
		return
	}

	// Verify the reward exists and its type matches the requested slot.
	rw, ok := s.rewardRegistry.Lookup(req.RewardID)
	if !ok {
		http.Error(w, fmt.Sprintf("%s: %s", gamification.ErrUnknownReward, req.RewardID), http.StatusNotFound)
		return
	}
	if rw.Type != slot {
		http.Error(w, fmt.Sprintf("slot mismatch: reward is %s, not %s", rw.Type, req.Slot), http.StatusBadRequest)
		return
	}

	loadout, err := s.tracker.Equip(s.rewardRegistry, req.RewardID)
	if err != nil {
		if errors.Is(err, gamification.ErrUnknownReward) {
			http.Error(w, "unknown reward", http.StatusNotFound)
			return
		}
		if errors.Is(err, gamification.ErrNotUnlocked) {
			http.Error(w, "reward not unlocked", http.StatusForbidden)
			return
		}
		http.Error(w, "equip failed", http.StatusInternalServerError)
		return
	}

	// Broadcast the change to all WebSocket clients.
	if msg, err := NewEquippedMessage(EquippedPayload{Loadout: loadout}); err != nil {
		slog.Error("equip marshal failed", "error", err)
	} else {
		s.broadcaster.BroadcastMessage(msg)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(loadout)
}

type unequipRequest struct {
	Slot string `json:"slot"`
}

func (s *Server) handleUnequip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.tracker == nil {
		http.Error(w, "stats not available", http.StatusServiceUnavailable)
		return
	}

	var req unequipRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if req.Slot == "" {
		http.Error(w, "slot is required", http.StatusBadRequest)
		return
	}

	slot := gamification.RewardType(req.Slot)
	if !gamification.ValidSlot(slot) {
		http.Error(w, "invalid slot", http.StatusBadRequest)
		return
	}

	loadout, err := s.tracker.Unequip(s.rewardRegistry, slot)
	if err != nil {
		http.Error(w, "unequip failed", http.StatusInternalServerError)
		return
	}

	if msg, err := NewEquippedMessage(EquippedPayload{Loadout: loadout}); err != nil {
		slog.Error("unequip marshal failed", "error", err)
	} else {
		s.broadcaster.BroadcastMessage(msg)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(loadout)
}

func (s *Server) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse: /api/sessions/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	sessionID, err := url.PathUnescape(parts[0])
	if err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "focus":
		s.handleFocus(w, r, sessionID)
	case "tail":
		s.handleTail(w, r, sessionID)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) handleFocus(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state, ok := s.store.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if state.TmuxTarget == "" {
		http.Error(w, "session has no tmux pane", http.StatusConflict)
		return
	}

	if err := tmuxFocusSession(state.TmuxTarget); err != nil {
		slog.Error("tmux focus failed", "session", sessionID, "target", state.TmuxTarget, "error", err)
		http.Error(w, "tmux focus failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTail(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	state, ok := s.store.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if state.LogPath == "" {
		http.Error(w, "session has no log file", http.StatusConflict)
		return
	}

	if err := session.ValidateLogPath(state.LogPath); err != nil {
		slog.Warn("tail: invalid log path", "session", sessionID, "error", err)
		http.Error(w, "invalid log path", http.StatusForbidden)
		return
	}

	var offset int64
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			offset = n
		}
	}

	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	entries, newOffset, err := session.ParseTailEntries(state.LogPath, offset, limit)
	if err != nil {
		slog.Error("tail parse failed", "session", sessionID, "error", err)
		http.Error(w, "failed to read log", http.StatusInternalServerError)
		return
	}

	resp := session.TailResponse{
		Entries: entries,
		Offset:  newOffset,
	}
	if resp.Entries == nil {
		resp.Entries = []session.TailEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) authorize(r *http.Request) bool {
	if s.authToken == "" {
		return true
	}
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && token == s.authToken
}

// Authorize is the exported form of authorize, for use by sub-handlers
// that need to validate the same auth token.
func (s *Server) Authorize(r *http.Request) bool {
	return s.authorize(r)
}

func (s *Server) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")

	if len(s.allowedOrigins) > 0 {
		// Explicit allowlist configured: require Origin header and match
		// against the configured origins.
		if origin == "" {
			return false
		}
		if s.allowedOrigins[origin] {
			return true
		}
		if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
			return s.allowedHosts[parsed.Host]
		}
		return false
	}

	// No allowlist configured — dev-only fallback.
	// Accepts same-origin, localhost, and missing Origin headers.
	// Configure allowed_origins in production to restrict access.
	if origin == "" {
		return true
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}

	host := parsed.Host
	if host == "" {
		return false
	}

	if host == r.Host {
		return true
	}

	// Dev-only: accept loopback addresses when no allowlist is configured.
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}

	return false
}

// securityHeaders wraps a handler to set standard HTTP security headers on all responses.
//
// Referrer-Policy: no-referrer prevents the browser from sending a Referer header on any
// request. This stops auth tokens embedded in URLs from leaking to third-party resources.
//
// Content-Security-Policy directives:
//   - default-src 'self': baseline — only same-origin resources allowed unless overridden below.
//   - connect-src 'self' ws://host wss://host: allows fetch/XHR to the same origin and WebSocket
//     connections only to the server's own host. The ws/wss origins are derived from the request's
//     Host header because 'self' alone does not reliably cover ws/wss across all browsers.
//   - style-src 'self' 'unsafe-inline': permits dynamically injected <style> elements (used by
//     the RewardSelector UI component). 'unsafe-inline' is required because the styles are created
//     at runtime without a nonce.
//   - img-src 'self' data:: allows same-origin images and data: URIs. Canvas drawImage() with
//     data: URLs and any canvas.toDataURL() output fall under this directive.
//   - object-src 'none': disables Flash/plugin embeds entirely.
//   - base-uri 'self': prevents <base> tag injection from redirecting relative URLs.
//
// Permissions-Policy disables browser features the app does not use (camera, microphone,
// geolocation, etc.) so they cannot be activated by injected scripts or iframes.
func securityHeaders(next http.Handler) http.Handler {
	const permissionsPolicy = "camera=(), " +
		"microphone=(), " +
		"geolocation=(), " +
		"payment=(), " +
		"usb=(), " +
		"magnetometer=(), " +
		"gyroscope=(), " +
		"accelerometer=()"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		wsOrigin := "ws://" + host
		wssOrigin := "wss://" + host

		csp := "default-src 'self'; " +
			"connect-src 'self' " + wsOrigin + " " + wssOrigin + "; " +
			"style-src 'self' 'unsafe-inline'; " +
			"img-src 'self' data:; " +
			"object-src 'none'; " +
			"base-uri 'self'"

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", permissionsPolicy)
		next.ServeHTTP(w, r)
	})
}

// decodeBody applies a MaxBytesReader limit, decodes JSON into dst, and writes
// the appropriate HTTP error response on failure. Returns true on success.
func decodeBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "invalid request body", http.StatusBadRequest)
		}
		return false
	}
	return true
}

func writeRateLimitExceeded(w http.ResponseWriter, retryAfter time.Duration) {
	seconds := int(math.Ceil(retryAfter.Seconds()))
	if seconds < 1 {
		seconds = 1
	}

	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
}

func NewHTTPServer(host string, port int, tls bool, mux *http.ServeMux) *http.Server {
	addr := fmt.Sprintf("%s:%d", host, port)
	handler := securityHeaders(mux)
	if tls {
		handler = hstsHeaders(handler)
	}
	return &http.Server{
		Addr:    addr,
		Handler: handler,
	}
}

// hstsHeaders adds a Strict-Transport-Security header to every response,
// telling browsers to only use HTTPS for future requests.
func hstsHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}
