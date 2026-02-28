package app

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard bindings for the TUI.
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	Enter        key.Binding
	Tab          key.Binding
	Zone1        key.Binding
	Zone2        key.Binding
	Zone3        key.Binding
	Escape       key.Binding
	Quit         key.Binding
	Achievements key.Binding
	Garage       key.Binding
	Debug        key.Binding
	BattlePass   key.Binding
	Resync       key.Binding
	Focus        key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/↑", "prev racer"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/↓", "next racer"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "detail / focus"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "cycle zone"),
		),
		Zone1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "racing zone"),
		),
		Zone2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "pit zone"),
		),
		Zone3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "parked zone"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close overlay"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Achievements: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "achievements"),
		),
		Garage: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "garage"),
		),
		Debug: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "debug log"),
		),
		BattlePass: key.NewBinding(
			key.WithKeys("b"),
			key.WithHelp("b", "battle pass"),
		),
		Resync: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "resync"),
		),
		Focus: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "focus tmux"),
		),
	}
}
