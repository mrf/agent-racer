package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
)

type Server struct {
	store           *session.Store
	broadcaster     *Broadcaster
	frontendDir     string
	dev             bool
	embeddedHandler http.Handler
	allowedOrigins  map[string]bool
	allowedHosts    map[string]bool
	authToken       string
}

func NewServer(store *session.Store, broadcaster *Broadcaster, frontendDir string, dev bool, embeddedHandler http.Handler, allowedOrigins []string, authToken string) *Server {
	s := &Server{
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

func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/api/sessions", s.handleSessions)

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
	sessions := s.store.GetAll()
	json.NewEncoder(w).Encode(sessions)
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
