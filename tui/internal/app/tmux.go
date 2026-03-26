package app

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
)

// validTmuxTarget matches a tmux target like "session:window.pane" where the
// session name contains only safe characters and window/pane are integers.
var validTmuxTarget = regexp.MustCompile(`^[a-zA-Z0-9_.-]+:\d+\.\d+$`)

// execTmuxSplit joins the session's tmux pane into the current window as a
// horizontal split alongside the TUI. Uses $TMUX_PANE to identify the caller's
// pane; the -d flag keeps focus on the TUI pane after the join.
func execTmuxSplit(sessionTarget string) error {
	if !validTmuxTarget.MatchString(sessionTarget) {
		return fmt.Errorf("invalid tmux target %q", sessionTarget)
	}
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	currentPane := os.Getenv("TMUX_PANE")
	if currentPane == "" {
		return fmt.Errorf("TMUX_PANE not set (not running in tmux)")
	}
	if err := exec.Command(tmuxPath, "join-pane", "-h", "-d", "-s", sessionTarget, "-t", currentPane).Run(); err != nil {
		return fmt.Errorf("join-pane: %w", err)
	}
	return nil
}

// tmuxInSession reports whether the process is running inside a tmux pane.
func tmuxInSession() bool {
	return os.Getenv("TMUX_PANE") != ""
}
