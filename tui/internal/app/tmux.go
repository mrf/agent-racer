package app

import (
	"fmt"
	"os"
	"os/exec"
)

// execTmuxSplit joins the session's tmux pane into the current window as a
// horizontal split alongside the TUI. Uses $TMUX_PANE to identify the caller's
// pane; the -d flag keeps focus on the TUI pane after the join.
func execTmuxSplit(sessionTarget string) error {
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
