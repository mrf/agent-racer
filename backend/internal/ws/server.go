package ws

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/agent-racer/backend/internal/session"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Server struct {
	store           *session.Store
	broadcaster     *Broadcaster
	frontendDir     string
	dev             bool
	embeddedHandler http.Handler
}

func NewServer(store *session.Store, broadcaster *Broadcaster, frontendDir string, dev bool, embeddedHandler http.Handler) *Server {
	return &Server{
		store:           store,
		broadcaster:     broadcaster,
		frontendDir:     frontendDir,
		dev:             dev,
		embeddedHandler: embeddedHandler,
	}
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
	w.Header().Set("Content-Type", "application/json")
	sessions := s.store.GetAll()
	json.NewEncoder(w).Encode(sessions)
}

func ListenAndServe(host string, port int, mux *http.ServeMux) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("Server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}
