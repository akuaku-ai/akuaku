// Package tui renders Akuaku's terminal user interface.
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/akuaku-ai/akuaku/internal/state"
)

// dash marks a value the monitor cannot know. Sessions reflected from Claude
// Code hooks report no token or cost usage, so both render as a dash rather than
// a misleading zero.
const dash = "—"

// tickInterval is how often the monitor refreshes its view.
const tickInterval = time.Second

// tickMsg is delivered on every refresh tick. It carries the tick time so the
// model never reads the clock itself, which keeps Update and View deterministic.
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

// runsMsg carries the result of scanning the state directory.
type runsMsg struct {
	runs []state.Run
	err  error
}

// loadRuns scans the state directory. It runs as a command so filesystem I/O
// stays out of the pure Update path.
func loadRuns() tea.Msg {
	runs, err := state.ReadDir(state.Dir())
	return runsMsg{runs: runs, err: err}
}

// Model is the root Bubble Tea model for the monitor. It holds only what it
// reads from the state directory; it never writes there. cursor is the selected
// row and detail toggles the single-run view.
type Model struct {
	runs   []state.Run
	now    time.Time
	err    error
	cursor int
	detail bool
}

// New returns a Model in its initial state.
func New() Model {
	return Model{}
}

// Init starts the refresh loop and loads the current runs.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), loadRuns)
}

// Update handles an incoming message and returns the next model and command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.runs)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.runs) > 0 {
				m.detail = true
			}
		case "esc":
			m.detail = false
		}
		return m, nil
	case tickMsg:
		m.now = time.Time(msg)
		return m, tea.Batch(tickCmd(), loadRuns)
	case runsMsg:
		m.runs = msg.runs
		m.err = msg.err
		m.cursor = clamp(m.cursor, len(m.runs))
		return m, nil
	}
	return m, nil
}

// clamp keeps the cursor within [0, length) so a shrinking run list never leaves
// it pointing past the end.
func clamp(cursor, length int) int {
	if cursor >= length {
		cursor = length - 1
	}
	if cursor < 0 {
		return 0
	}
	return cursor
}

// duration reports how long a run has been active: live (now - started) while
// running or when no end time is recorded, and frozen (ended - started) once the
// run has finished.
func duration(run state.Run, now time.Time) time.Duration {
	if run.Status == state.StatusRunning || run.EndedAt == nil {
		return now.Sub(run.StartedAt)
	}
	return run.EndedAt.Sub(run.StartedAt)
}

// formatDuration renders a duration as M:SS, clamping negatives to zero.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%d:%02d", int(d/time.Minute), int(d/time.Second)%60)
}

// formatTokens renders a run's token count, or a dash for reflected sessions
// whose usage Akuaku cannot observe.
func formatTokens(run state.Run) string {
	if run.Source == "hook" {
		return dash
	}
	return strconv.Itoa(run.Tokens)
}

// formatCost renders a run's cost, or a dash for reflected sessions whose usage
// Akuaku cannot observe.
func formatCost(run state.Run) string {
	if run.Source == "hook" {
		return dash
	}
	return fmt.Sprintf("$%.2f", run.Cost)
}

// metrics holds the aggregate counters derived from the current runs.
type metrics struct {
	running int
	done    int
	errored int
	tokens  int
	cost    float64
}

// computeMetrics derives the dashboard counters from runs.
func computeMetrics(runs []state.Run) metrics {
	var m metrics
	for _, run := range runs {
		switch run.Status {
		case state.StatusRunning:
			m.running++
		case state.StatusDone:
			m.done++
		case state.StatusError:
			m.errored++
		}
		m.tokens += run.Tokens
		m.cost += run.Cost
	}
	return m
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	headerStyle = lipgloss.NewStyle().Faint(true)
)

// View renders the current frame: the single-run detail when a run is selected
// and open, otherwise the list of agents.
func (m Model) View() string {
	if m.detail && len(m.runs) > 0 {
		return m.detailView()
	}
	return m.listView()
}

// listView renders the title, an optional error, the agent table (or an
// empty-state hint) with the selected row marked, and the metrics panel.
func (m Model) listView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Akuaku 🗿"))
	b.WriteString("\n\n")

	if m.err != nil {
		fmt.Fprintf(&b, "error reading state: %s\n\n", m.err)
	}

	if len(m.runs) == 0 {
		b.WriteString("no agents yet — launch one with `akuaku run`\n\n")
	} else {
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-20s %-8s %-8s %8s %8s %8s",
			"NAME", "BACKEND", "STATUS", "DUR", "TOKENS", "COST")))
		b.WriteByte('\n')
		for i, run := range m.runs {
			marker := "  "
			if i == m.cursor {
				marker = "> "
			}
			fmt.Fprintf(&b, "%s%-20.20s %-8s %-8s %8s %8s %8s\n",
				marker, run.Name, run.Backend, run.Status,
				formatDuration(duration(run, m.now)),
				formatTokens(run), formatCost(run))
		}
		b.WriteByte('\n')
	}

	mt := computeMetrics(m.runs)
	fmt.Fprintf(&b, "running: %d  ok: %d  err: %d  tokens: %d  cost: $%.2f\n",
		mt.running, mt.done, mt.errored, mt.tokens, mt.cost)
	b.WriteString(headerStyle.Render("↑/↓ move · enter details · q quit"))
	b.WriteByte('\n')
	return b.String()
}

// detailView renders the selected run: its metadata, its usage, and either the
// captured answer, the failure reason, or a note that no output was recorded.
func (m Model) detailView() string {
	run := m.runs[m.cursor]

	var b strings.Builder
	b.WriteString(titleStyle.Render("Akuaku 🗿 — " + run.Name))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "backend:  %s\n", run.Backend)
	fmt.Fprintf(&b, "status:   %s\n", run.Status)
	if run.Task != "" {
		fmt.Fprintf(&b, "task:     %s\n", run.Task)
	}
	fmt.Fprintf(&b, "tokens:   %s\n", formatTokens(run))
	fmt.Fprintf(&b, "cost:     %s\n\n", formatCost(run))

	switch {
	case run.Status == state.StatusError && run.Error != "":
		b.WriteString(run.Error)
	case run.Output != "":
		b.WriteString(run.Output)
	default:
		b.WriteString(headerStyle.Render("(no output captured)"))
	}
	b.WriteString("\n\n")
	b.WriteString(headerStyle.Render("esc back · q quit"))
	b.WriteByte('\n')
	return b.String()
}
