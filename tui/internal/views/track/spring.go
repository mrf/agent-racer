package track

import (
	"math"
	"time"

	"github.com/charmbracelet/harmonica"
	tea "github.com/charmbracelet/bubbletea"
)

const animFPS = 30

// AnimTickMsg is dispatched each animation frame to advance spring physics.
type AnimTickMsg struct{}

// TickCmd schedules the next animation frame.
func TickCmd() tea.Cmd {
	return tea.Tick(time.Second/animFPS, func(time.Time) tea.Msg {
		return AnimTickMsg{}
	})
}

// barSpring animates a context utilization value with spring physics.
type barSpring struct {
	sp     harmonica.Spring
	pos    float64
	vel    float64
	target float64
}

func newBarSpring(initial float64) *barSpring {
	return &barSpring{
		sp:     harmonica.NewSpring(harmonica.FPS(animFPS), 5.0, 0.85),
		pos:    initial,
		target: initial,
	}
}

func (b *barSpring) setTarget(t float64) {
	b.target = t
}

func (b *barSpring) step() {
	b.pos, b.vel = b.sp.Update(b.pos, b.vel, b.target)
}

func (b *barSpring) atRest() bool {
	return math.Abs(b.vel) < 0.001 && math.Abs(b.pos-b.target) < 0.005
}
