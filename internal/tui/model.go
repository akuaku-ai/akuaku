// Package tui renders Akuaku's terminal user interface.
package tui

import (
	"fmt"
	"sort"
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
// row, detail toggles the single-run view, and width/height track the terminal
// size so the dashboard fills it.
type Model struct {
	runs   []state.Run
	now    time.Time
	err    error
	cursor int
	detail bool
	width  int
	height int
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
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tickMsg:
		m.now = time.Time(msg)
		return m, tea.Batch(tickCmd(), loadRuns)
	case runsMsg:
		sortRuns(msg.runs)
		m.runs = msg.runs
		m.err = msg.err
		m.cursor = clamp(m.cursor, len(m.runs))
		return m, nil
	}
	return m, nil
}

// sortRuns orders runs so live ones lead the list and, within each group, the
// most recently started appears first.
func sortRuns(runs []state.Run) {
	sort.SliceStable(runs, func(i, j int) bool {
		if ri, rj := runningRank(runs[i].Status), runningRank(runs[j].Status); ri != rj {
			return ri < rj
		}
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
}

// runningRank sorts running runs ahead of every terminal state.
func runningRank(s state.Status) int {
	if s == state.StatusRunning {
		return 0
	}
	return 1
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

// The dashboard palette: a stone-and-aqua signature with status-driven row color.
var (
	colorAccent   = lipgloss.Color("44")  // aqua — the Akuaku accent
	colorStone    = lipgloss.Color("240") // border gray
	colorRunning  = lipgloss.Color("42")  // green — live
	colorError    = lipgloss.Color("203") // red — failed
	colorDone     = lipgloss.Color("246") // dim gray — finished
	colorSelected = lipgloss.Color("236") // subtle highlight

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	headerStyle = lipgloss.NewStyle().Faint(true)
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorStone).Padding(0, 1)
	appStyle    = lipgloss.NewStyle().Padding(outerPadY, outerPadX)
)

// Fixed table column widths; the name column flexes up to maxNameW.
const (
	backendW  = 8
	modelW    = 15
	durW      = 5
	tokensW   = 8
	costW     = 8
	minNameW  = 10
	maxNameW  = 36
	minInner  = 20
	fixedCols = 2 + 6 + backendW + modelW + durW + tokensW + costW // marker+glyph, six gaps, fixed columns
	// maxDashW caps how wide the dashboard grows so an ultra-wide terminal does
	// not stretch it into a sea of empty space; the box border and padding add 4.
	maxDashW = fixedCols + maxNameW + 4
	// outerPadX/Y frame the whole dashboard so it never sits flush to the edges.
	outerPadX = 2
	outerPadY = 1
)

// View renders the current frame: the single-run detail when a run is selected
// and open, otherwise the full-width dashboard.
func (m Model) View() string {
	if m.detail && len(m.runs) > 0 {
		return m.detailView()
	}
	return m.listView()
}

// listView renders the dashboard: an overview strip, the agent table, and a
// keybinding footer, capped in width and framed by outer padding.
func (m Model) listView() string {
	width := m.width - 2*outerPadX
	if width <= 0 {
		width = 80
	}
	if width > maxDashW {
		width = maxDashW
	}
	// Shrink to the widest name so short lists stay dense instead of leaving a
	// gap between the name and the metric columns.
	nameW := longestName(m.runs)
	if nameW < minNameW {
		nameW = minNameW
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}
	if needed := fixedCols + nameW + 4; needed < width {
		width = needed
	}

	sections := []string{
		m.header(width, computeMetrics(m.runs)),
		m.table(width),
		headerStyle.Render("↑/↓ move · enter open · q quit"),
	}
	return appStyle.Render(strings.Join(sections, "\n"))
}

// longestName is the rune length of the widest run name, sizing the name column.
func longestName(runs []state.Run) int {
	n := 0
	for _, run := range runs {
		if l := len([]rune(run.Name)); l > n {
			n = l
		}
	}
	return n
}

// header is the k9s-style top strip: run stats on the left and the Akuaku logo
// right-aligned. The logo is the brand mark that replaces the emoji.
func (m Model) header(width int, mt metrics) string {
	logo := logoBlock()
	logoW := 0
	for _, art := range logo {
		if w := lipgloss.Width(art); w > logoW {
			logoW = w
		}
	}

	left := []string{
		fmt.Sprintf("running %d · done %d · err %d", mt.running, mt.done, mt.errored),
		fmt.Sprintf("%s tokens · $%.2f", humanizeTokens(mt.tokens), mt.cost),
		"● live",
	}

	var b strings.Builder
	for i, art := range logo {
		stat := ""
		if i < len(left) {
			stat = left[i]
		}
		gap := width - logoW - lipgloss.Width(stat)
		if gap < 1 {
			gap = 1
		}
		b.WriteString(stat)
		b.WriteString(strings.Repeat(" ", gap))
		b.WriteString(art)
		if i < len(logo)-1 {
			b.WriteByte('\n')
		}
	}
	if m.err != nil {
		fmt.Fprintf(&b, "\nerror reading state: %s", m.err)
	}
	return b.String()
}

// logoBlock is the Akuaku brand mark: a colored tiki mask beside the wordmark in
// block characters over a feathered signature. Each line is pre-styled, so the
// caller measures it with lipgloss.Width.
func logoBlock() []string {
	word := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	rows := []string{
		"▄▀█ █▄▀ █ █ ▄▀█ █▄▀ █ █",
		"█▀█ █▀▄ █▄█ █▀█ █▀▄ █▄█",
		`\|/ akuaku \|/`,
	}
	mask := maskLines()
	block := make([]string, len(rows))
	for i := range rows {
		block[i] = mask[i] + "  " + word.Render(rows[i])
	}
	return block
}

// maskLines draws a small, colorful tiki mask (feathers, eyes, mouth) — Akuaku's
// face. Every line is five display columns wide so it aligns beside the wordmark.
func maskLines() []string {
	paint := func(code, s string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(code)).Render(s)
	}
	return []string{
		" " + paint("99", `\`) + paint("220", "|") + paint("208", "/") + " ",
		paint("130", "(") + paint("220", "●") + " " + paint("220", "●") + paint("130", ")"),
		" " + paint("196", "╰—╯") + " ",
	}
}

// table renders the bordered agent list sized to width, with the selected row
// marked and each row colored by status.
func (m Model) table(width int) string {
	innerW := width - 4
	if innerW < minInner {
		innerW = minInner
	}
	nameW := innerW - fixedCols
	if nameW < minNameW {
		nameW = minNameW
	}

	var b strings.Builder
	live := headerStyle.Render("● live")
	b.WriteString(padRight(fmt.Sprintf("Agents (%d)", len(m.runs)), innerW-lipgloss.Width(live)))
	b.WriteString(live)

	if len(m.runs) == 0 {
		b.WriteString("\nno agents yet — launch one with `akuaku run`")
	} else {
		b.WriteByte('\n')
		b.WriteString(headerStyle.Render(formatRow(" ", " ", "NAME", "BACKEND", "MODEL", "DUR", "TOKENS", "COST", nameW)))
		for i, run := range m.runs {
			marker := " "
			if i == m.cursor {
				marker = ">"
			}
			row := formatRow(marker, statusGlyph(run.Status), run.Name, run.Backend, run.Model,
				formatDuration(duration(run, m.now)), formatTokens(run), formatCost(run), nameW)
			b.WriteByte('\n')
			b.WriteString(rowStyle(run.Status, i == m.cursor).Render(row))
		}
	}
	return boxStyle.Width(width - 2).Render(b.String())
}

// formatRow lays a run's cells into fixed columns; name flexes to nameW.
func formatRow(marker, glyph, name, backend, model, dur, tokens, cost string, nameW int) string {
	return fmt.Sprintf("%s%s %s %s %s %s %s %s",
		marker, glyph,
		padRight(name, nameW), padRight(backend, backendW), padRight(model, modelW),
		padLeft(dur, durW), padLeft(tokens, tokensW), padLeft(cost, costW))
}

// rowStyle colors a row by status and highlights the selected one.
func rowStyle(status state.Status, selected bool) lipgloss.Style {
	style := lipgloss.NewStyle()
	switch status {
	case state.StatusRunning:
		style = style.Foreground(colorRunning)
	case state.StatusError:
		style = style.Foreground(colorError)
	default:
		style = style.Foreground(colorDone)
	}
	if selected {
		style = style.Background(colorSelected).Bold(true)
	}
	return style
}

// statusGlyph is the leading status marker for a run.
func statusGlyph(s state.Status) string {
	switch s {
	case state.StatusRunning:
		return "●"
	case state.StatusError:
		return "✖"
	default:
		return "✔"
	}
}

// humanizeTokens renders large token counts compactly (1.5k, 2.5M).
func humanizeTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return strconv.Itoa(n)
	}
}

// padRight left-aligns s in a field of w runes, truncating if it overflows.
func padRight(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

// padLeft right-aligns s in a field of w runes, truncating if it overflows.
func padLeft(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w])
	}
	return strings.Repeat(" ", w-len(r)) + s
}

// detailView renders the selected run: its metadata, its usage, and either the
// captured answer, the failure reason, or a note that no output was recorded.
func (m Model) detailView() string {
	run := m.runs[m.cursor]

	var b strings.Builder
	b.WriteString(titleStyle.Render("akuaku") + "  " + lipgloss.NewStyle().Bold(true).Render(run.Name))
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
	return appStyle.Render(b.String())
}
