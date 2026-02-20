package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"strings"

	"github.com/agent-racer/backend/internal/config"
	"github.com/agent-racer/backend/internal/gamification"
	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
)

// tmuxFocusSession switches to the tmux pane identified by target (e.g. "main:2.0").
func tmuxFocusSession(target string) error {
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
	config          *config.Config
	store           *session.Store
	broadcaster     *Broadcaster
	frontendDir     string
	dev             bool
	embeddedHandler http.Handler
	allowedOrigins  map[string]bool
	allowedHosts    map[string]bool
	authToken       string
	tracker         *gamification.StatsTracker
}

func NewServer(cfg *config.Config, store *session.Store, broadcaster *Broadcaster, frontendDir string, dev bool, embeddedHandler http.Handler, allowedOrigins []string, authToken string) *Server {
	s := &Server{
		config:          cfg,
		store:           store,
		broadcaster:     broadcaster,
		frontendDir:     frontendDir,
		dev:             dev,
		embeddedHandler: embeddedHandler,
		allowedOrigins:  make(map[string]bool),
		allowedHosts:    make(map[string]bool),
		authToken:       authToken,
	}

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

// SetStatsTracker configures the stats tracker used by the /api/stats endpoint.
// Must be called before SetupRoutes.
func (s *Server) SetStatsTracker(tracker *gamification.StatsTracker) {
	s.tracker = tracker
}

func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/", s.handleSessionRoutes)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/stats", s.handleStats)

	if s.dev {
		log.Printf("Serving frontend from filesystem: %s", s.frontendDir)
		mux.Handle("/", http.FileServer(http.Dir(s.frontendDir)))
	} else if s.embeddedHandler != nil {
		log.Println("Serving embedded frontend")
		mux.Handle("/", s.embeddedHandler)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: s.checkOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	log.Printf("WebSocket client connected: %s", r.RemoteAddr)
	c := s.broadcaster.AddClient(conn)

	go func() {
		defer func() {
			s.broadcaster.RemoveClient(c)
			log.Printf("WebSocket client disconnected: %s", r.RemoteAddr)
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	sessions := s.broadcaster.FilterSessions(s.store.GetAll())
	json.NewEncoder(w).Encode(sessions)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.config.Sound)
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
	json.NewEncoder(w).Encode(s.tracker.Stats())
}

func (s *Server) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse: /api/sessions/{id}/focus
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "focus" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	sessionID, err := url.PathUnescape(parts[0])
	if err != nil {
		http.Error(w, "invalid session id", http.StatusBadRequest)
		return
	}
	s.handleFocus(w, r, sessionID)
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
		http.Error(w, fmt.Sprintf("tmux focus failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authorize(r *http.Request) bool {
	if s.authToken == "" {
		return true
	}

	if r.URL.Query().Get("token") == s.authToken {
		return true
	}

	if r.Header.Get("X-Agent-Racer-Token") == s.authToken {
		return true
	}

	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") && strings.TrimPrefix(auth, "Bearer ") == s.authToken {
		return true
	}

	return false
}

func (s *Server) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	if len(s.allowedOrigins) > 0 {
		if s.allowedOrigins[origin] {
			return true
		}
		if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
			return s.allowedHosts[parsed.Host]
		}
		return false
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

	if strings.HasPrefix(host, "localhost:") || host == "localhost" {
		return true
	}
	if strings.HasPrefix(host, "127.0.0.1:") || host == "127.0.0.1" {
		return true
	}
	if strings.HasPrefix(host, "[::1]:") || host == "::1" {
		return true
	}

	return false
}

func ListenAndServe(host string, port int, mux *http.ServeMux) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("Server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}
