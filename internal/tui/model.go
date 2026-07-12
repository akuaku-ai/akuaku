// Package tui renders Akuaku's terminal user interface.
package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// tickInterval is how often the monitor refreshes its view.
const tickInterval = time.Second

// tickMsg is delivered on every refresh tick. It carries the tick time so the
// model never reads the clock itself, which keeps Update and View deterministic
// and therefore easy to test.
type tickMsg time.Time

// newTickMsg wraps a time as a tickMsg. It is a named function rather than an
// inline closure so it can be unit-tested without waiting for a real tick.
func newTickMsg(t time.Time) tea.Msg {
	return tickMsg(t)
}

// tickCmd schedules the next refresh tick.
func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, newTickMsg)
}

// Model is the root Bubble Tea model for the monitor.
type Model struct {
	ticks int
}

// New returns a Model in its initial state.
func New() Model {
	return Model{}
}

// Init starts the refresh loop.
func (m Model) Init() tea.Cmd {
	return tickCmd()
}

// Update handles an incoming message and returns the next model and command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		m.ticks++
		return m, tickCmd()
	}
	return m, nil
}

var titleStyle = lipgloss.NewStyle().Bold(true)

// View renders the current frame.
func (m Model) View() string {
	title := titleStyle.Render("Akuaku 🗿")
	return fmt.Sprintf("%s\n\nalive — ticks: %d\n\npress q to quit\n", title, m.ticks)
}
