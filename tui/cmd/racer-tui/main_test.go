package main

import (
	"bytes"
	"testing"
)

func TestParseArgsVersionFlag(t *testing.T) {
	var stderr bytes.Buffer

	opts, err := parseArgs([]string{"--version"}, &stderr)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if !opts.showVersion {
		t.Fatal("showVersion = false, want true")
	}
}

func TestPrintVersion(t *testing.T) {
	originalVersion := version
	version = "test-version"
	t.Cleanup(func() {
		version = originalVersion
	})

	var stdout bytes.Buffer
	if err := printVersion(&stdout); err != nil {
		t.Fatalf("printVersion returned error: %v", err)
	}

	if got := stdout.String(); got != "test-version\n" {
		t.Fatalf("printVersion() = %q, want %q", got, "test-version\n")
	}
}

func TestDeriveHTTPBase(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "websocket to http",
			input:  "ws://127.0.0.1:8080/ws",
			output: "http://127.0.0.1:8080",
		},
		{
			name:   "secure websocket to https",
			input:  "wss://example.com/ws",
			output: "https://example.com",
		},
		{
			name:   "invalid url falls back to default",
			input:  "://bad",
			output: "http://127.0.0.1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveHTTPBase(tt.input); got != tt.output {
				t.Fatalf("deriveHTTPBase(%q) = %q, want %q", tt.input, got, tt.output)
			}
		})
	}
}
