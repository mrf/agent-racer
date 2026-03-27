package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/agent-racer/tui/internal/app"
	"github.com/agent-racer/tui/internal/client"
	"github.com/agent-racer/tui/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "dev"

type cliOptions struct {
	configPath  string
	wsURL       string
	token       string
	showVersion bool
}

func parseArgs(args []string, output io.Writer) (cliOptions, error) {
	var opts cliOptions

	fs := flag.NewFlagSet("agent-racer", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.configPath, "config", "", "Path to config file (defaults to ~/.config/agent-racer/config.yaml)")
	fs.StringVar(&opts.wsURL, "url", "", "WebSocket URL of the Agent Racer backend (overrides config)")
	fs.StringVar(&opts.token, "token", "", "Auth token (overrides config)")
	fs.BoolVar(&opts.showVersion, "version", false, "Print version information and exit")

	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}

	return opts, nil
}

func printVersion(output io.Writer) error {
	_, err := fmt.Fprintln(output, version)
	return err
}

func main() {
	opts, err := parseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	if opts.showVersion {
		if err := printVersion(os.Stdout); err != nil {
			os.Exit(1)
		}
		return
	}

	// Load configuration from file.
	cfgPath := opts.configPath
	if cfgPath == "" {
		cfgPath = config.DefaultConfigPath()
	}
	cfg, cfgWarn := config.LoadOrDefault(cfgPath)
	if cfgWarn != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", cfgWarn)
	}

	// Resolve WebSocket URL: CLI flag > config file > hardcoded default.
	effectiveURL := cfg.WebSocketURL()
	if opts.wsURL != "" {
		effectiveURL = opts.wsURL
	}

	// Resolve auth token: CLI flag > config file.
	effectiveToken := cfg.Server.AuthToken
	if opts.token != "" {
		effectiveToken = opts.token
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
