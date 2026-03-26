package ws

import "testing"

func TestValidTmuxTarget(t *testing.T) {
	valid := []string{
		"main:0.0",
		"main:2.0",
		"dev:1.3",
		"my-session:10.5",
		"my_session:0.0",
		"cc-fix-bug:0.0",
		"dots.in.name:1.2",
	}
	for _, target := range valid {
		if !validTmuxTarget.MatchString(target) {
			t.Errorf("expected valid target %q to match", target)
		}
	}

	invalid := []string{
		"",
		"main",
		"main:0",
		":0.0",
		"main:0.0; rm -rf /",
		"main:0.0\nselect-pane -t evil",
		"$(whoami):0.0",
		"`id`:0.0",
		"main:0.0 -t evil",
		"foo bar:0.0",
		"main:0.0\x00",
	}
	for _, target := range invalid {
		if validTmuxTarget.MatchString(target) {
			t.Errorf("expected invalid target %q to NOT match", target)
		}
	}
}

func TestTmuxFocusSession_RejectsInvalidTarget(t *testing.T) {
	err := tmuxFocusSession("$(whoami):0.0")
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
	if got := err.Error(); got != `invalid tmux target "$(whoami):0.0"` {
		t.Errorf("unexpected error: %s", got)
	}
}
