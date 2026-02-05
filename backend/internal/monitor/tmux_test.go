package monitor

import (
	"testing"
)

func TestParseTmuxPanes(t *testing.T) {
	input := "1234\tmain\t0\t0\n5678\tmain\t1\t0\n9012\tdev\t2\t1\n"

	panes := parseTmuxPanes(input)
	if len(panes) != 3 {
		t.Fatalf("expected 3 panes, got %d", len(panes))
	}

	tests := []struct {
		idx         int
		sessionName string
		windowIndex int
		paneIndex   int
		panePID     int
		target      string
	}{
		{0, "main", 0, 0, 1234, "main:0.0"},
		{1, "main", 1, 0, 5678, "main:1.0"},
		{2, "dev", 2, 1, 9012, "dev:2.1"},
	}

	for _, tt := range tests {
		p := panes[tt.idx]
		if p.SessionName != tt.sessionName {
			t.Errorf("pane %d: session=%q, want %q", tt.idx, p.SessionName, tt.sessionName)
		}
		if p.WindowIndex != tt.windowIndex {
			t.Errorf("pane %d: window=%d, want %d", tt.idx, p.WindowIndex, tt.windowIndex)
		}
		if p.PaneIndex != tt.paneIndex {
			t.Errorf("pane %d: pane=%d, want %d", tt.idx, p.PaneIndex, tt.paneIndex)
		}
		if p.PanePID != tt.panePID {
			t.Errorf("pane %d: pid=%d, want %d", tt.idx, p.PanePID, tt.panePID)
		}
		if p.Target != tt.target {
			t.Errorf("pane %d: target=%q, want %q", tt.idx, p.Target, tt.target)
		}
	}
}

func TestParseTmuxPanes_EmptyAndMalformed(t *testing.T) {
	// Empty input
	panes := parseTmuxPanes("")
	if len(panes) != 0 {
		t.Errorf("empty input: expected 0 panes, got %d", len(panes))
	}

	// Malformed lines (wrong field count, bad ints)
	input := "notanumber\tmain\t0\t0\n1234\tmain\tbad\t0\n1234\t0\t0\n"
	panes = parseTmuxPanes(input)
	if len(panes) != 0 {
		t.Errorf("malformed input: expected 0 panes, got %d", len(panes))
	}
}

func TestParseParentPID(t *testing.T) {
	tests := []struct {
		name string
		stat string
		want int
	}{
		{
			name: "normal process",
			stat: "5678 (node) S 1234 5678 5678 0 -1 4194304 ...",
			want: 1234,
		},
		{
			name: "comm with spaces",
			stat: "5678 (my process) S 1234 5678 5678 0 -1 4194304 ...",
			want: 1234,
		},
		{
			name: "comm with parens",
			stat: "5678 (my (proc)) S 1234 5678 5678 0 -1 4194304 ...",
			want: 1234,
		},
		{
			name: "empty string",
			stat: "",
			want: 0,
		},
		{
			name: "no closing paren",
			stat: "5678 (node S 1234",
			want: 0,
		},
		{
			name: "truncated after paren",
			stat: "5678 (node)",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseParentPID(tt.stat)
			if got != tt.want {
				t.Errorf("parseParentPID(%q) = %d, want %d", tt.stat, got, tt.want)
			}
		})
	}
}

func TestResolve_DirectChild(t *testing.T) {
	resolver := &TmuxResolver{
		targetByPID: map[int]string{
			100: "main:0.0",
			200: "main:1.0",
		},
	}

	// Direct match (the PID itself is a pane PID)
	target, ok := resolver.Resolve(100)
	if !ok || target != "main:0.0" {
		t.Errorf("Resolve(100) = (%q, %v), want (\"main:0.0\", true)", target, ok)
	}
}

func TestResolve_NoMatch(t *testing.T) {
	resolver := &TmuxResolver{
		targetByPID: map[int]string{
			100: "main:0.0",
		},
	}

	// PID 999 won't match any pane and getParentPID will fail (no /proc/999)
	target, ok := resolver.Resolve(999)
	if ok {
		t.Errorf("Resolve(999) = (%q, true), want (\"\", false)", target)
	}
}

func TestResolve_NilResolver(t *testing.T) {
	var resolver *TmuxResolver
	target, ok := resolver.Resolve(100)
	if ok {
		t.Errorf("nil resolver: Resolve(100) = (%q, true), want (\"\", false)", target)
	}
}
