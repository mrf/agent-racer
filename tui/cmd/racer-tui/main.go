package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/agent-racer/tui/internal/app"
	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	configPath := flag.String("config", "", "Path to config file (defaults to ~/.config/agent-racer/config.yaml)")
	wsURL := flag.String("url", "", "WebSocket URL of the Agent Racer backend (overrides config)")
	token := flag.String("token", "", "Auth token (overrides config)")
	flag.Parse()

	// Load configuration from file.
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}
	cfg := config.LoadOrDefault(cfgPath)

	// Resolve WebSocket URL: CLI flag > config file > hardcoded default.
	effectiveURL := cfg.WebSocketURL()
	if *wsURL != "" {
		effectiveURL = *wsURL
	}

	// Resolve auth token: CLI flag > config file.
	effectiveToken := cfg.Server.AuthToken
	if *token != "" {
		effectiveToken = *token
	}

	// Derive HTTP base URL from WebSocket URL.
	httpBase := deriveHTTPBase(effectiveURL)

	ws := client.NewWSClient(effectiveURL, effectiveToken)
	httpClient := client.NewHTTPClient(httpBase, effectiveToken)

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
