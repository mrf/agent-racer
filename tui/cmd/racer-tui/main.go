package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/agent-racer/tui/internal/app"
	"github.com/agent-racer/tui/internal/client"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	wsURL := flag.String("url", "ws://127.0.0.1:8080/ws", "WebSocket URL of the Agent Racer backend")
	token := flag.String("token", "", "Auth token (if backend requires it)")
	flag.Parse()

	// Derive HTTP base URL from WebSocket URL.
	httpBase := deriveHTTPBase(*wsURL)

	ws := client.NewWSClient(*wsURL, *token)
	httpClient := client.NewHTTPClient(httpBase, *token)

	m := app.New(ws, httpClient)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// deriveHTTPBase converts ws://host:port/ws → http://host:port
func deriveHTTPBase(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	scheme := "http"
	if strings.HasPrefix(u.Scheme, "wss") {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, u.Host)
}
