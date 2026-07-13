// Package tui renders Akuaku's terminal user interface.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/akuaku-ai/akuaku/internal/brand"
	"github.com/akuaku-ai/akuaku/internal/state"
)

// getwd resolves the monitor's working directory, the root of the local scope.
// It is a seam so scope filtering can be tested with a fixed root.
var getwd = os.Getwd

// dash marks a value the monitor cannot know. Sessions reflected from Claude
// Code hooks report no token or cost usage, so both render as a dash rather than
// a misleading zero.
const dash = "—"

// tickInterval is how often the monitor refreshes its runs from disk.
const tickInterval = time.Second

// spinnerInterval is how often the running spinner advances. It is faster than
// the refresh so animation stays smooth without re-reading the state directory.
const spinnerInterval = 120 * time.Millisecond

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

// spinnerMsg is delivered on every animation frame.
type spinnerMsg struct{}

// newSpinnerMsg wraps a time as a spinnerMsg. Named so the spinner can be tested
// without waiting for a real frame.
func newSpinnerMsg(time.Time) tea.Msg { return spinnerMsg{} }

// spinnerCmd schedules the next animation frame.
func spinnerCmd() tea.Cmd {
	return tea.Tick(spinnerInterval, newSpinnerMsg)
}

// runsMsg carries the result of scanning the state directory.
type runsMsg struct {
	runs []state.Run
	err  error
}

// loadRuns scans the state directory and applies the custom-name overlay. When
// discover is set it also merges the running agent processes so sessions started
// before the monitor opened appear. It runs as a command so filesystem and
// process I/O stays out of the pure Update path.
func loadRuns(discover bool) tea.Msg {
	dir := state.Dir()
	runs, err := state.ReadDir(dir)
	names := state.ReadNames(dir)
	for i := range runs {
		if name, ok := names[runs[i].ID]; ok {
			runs[i].Name = name
		}
	}
	if discover {
		runs = mergeDiscovered(runs, listProcesses())
	}
	return runsMsg{runs: runs, err: err}
}

// listProcesses yields the currently running agent processes as runs. It is a
// seam: the binary wires it to a gopsutil-backed scan, tests inject fixtures,
// and the default returns nothing so discovery stays inert until it is enabled.
var listProcesses = func() []state.Run { return nil }

// SetProcessSource wires the process scanner the monitor merges in for
// `:discovery`. The binary calls it once at startup.
func SetProcessSource(f func() []state.Run) { listProcesses = f }

// mergeDiscovered appends discovered runs whose process is not already on disk,
// deduped by PID so an agent Akuaku launched (which records its PID) is never
// shown twice.
func mergeDiscovered(onDisk, discovered []state.Run) []state.Run {
	seen := make(map[int]bool, len(onDisk))
	for _, run := range onDisk {
		if run.PID != 0 {
			seen[run.PID] = true
		}
	}
	for _, run := range discovered {
		if !seen[run.PID] {
			onDisk = append(onDisk, run)
		}
	}
	return onDisk
}

// Model is the root Bubble Tea model for the monitor. It holds only what it
// reads from the state directory; it never writes there. cursor is the selected
// row, detail toggles the single-run view, and width/height track the terminal
// size so the dashboard fills it.
type Model struct {
	runs        []state.Run
	now         time.Time
	err         error
	cursor      int
	detail      bool
	width       int
	height      int
	filter      string // active filter query; empty shows everything
	filtering   bool   // whether the filter input is being edited
	showAll     bool   // false shows only running agents; true shows the full history
	discover    bool   // whether running processes are scanned and merged in
	global      bool   // false scopes the list to root; true shows every directory
	root        string // the monitor's working directory: the local scope's root
	command     string // command being typed after ":"
	commanding  bool   // whether the command input is being edited
	commandMsg  string // result of the last command, shown until the next one
	confirmKill bool   // whether the k-key kill is awaiting y/n confirmation
	killPID     int    // the process the armed kill will signal
	killName    string // the armed kill's target name, for the prompt

	lastStatus map[string]state.Status // each run's status at the previous load, to detect transitions
	alert      string                  // the most recent attention banner, cleared on the next keypress
	frame      int                     // animation frame, advanced by the spinner tick
}

// New returns a Model in its initial state, with process discovery on so
// sessions already running when the monitor opens appear without any setup, and
// the list scoped to the directory Akuaku was launched in.
func New() Model {
	root, _ := getwd()
	return Model{discover: true, root: root}
}

// Init starts the refresh loop, the spinner animation, and the first load.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), spinnerCmd(), m.reload())
}

// reload scans the state directory — and, when discovery is on, running
// processes — off the Update path. It captures the flag so the async command
// reflects the setting in force when it was scheduled.
func (m Model) reload() tea.Cmd {
	discover := m.discover
	return func() tea.Msg { return loadRuns(discover) }
}

// Update handles an incoming message and returns the next model and command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filtering {
			return m.filterKey(msg)
		}
		if m.commanding {
			return m.commandKey(msg)
		}
		if m.confirmKill {
			return m.confirmKillKey(msg)
		}
		m.alert = "" // any interaction dismisses the attention banner
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.visible())-1 {
				m.cursor++
			}
		case "k":
			if !m.detail {
				m = m.armKill()
			}
		case "enter":
			if !m.detail && len(m.visible()) > 0 {
				m.detail = true
			}
		case "esc":
			m.detail = false
		case "/":
			if !m.detail {
				m.filtering = true
			}
		case "a":
			if !m.detail {
				m.showAll = !m.showAll
				m.cursor = clamp(m.cursor, len(m.visible()))
			}
		case ":":
			m.commanding = true
			m.commandMsg = ""
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case spinnerMsg:
		m.frame++
		return m, spinnerCmd()
	case tickMsg:
		m.now = time.Time(msg)
		return m, tea.Batch(tickCmd(), m.reload())
	case runsMsg:
		events := attentionEvents(m.lastStatus, msg.runs)
		m.lastStatus = statusIndex(msg.runs)
		sortRuns(msg.runs)
		m.runs = msg.runs
		m.err = msg.err
		m.cursor = clamp(m.cursor, len(m.visible()))
		if len(events) > 0 {
			m.alert = events[len(events)-1]
			return m, bellCmd()
		}
		return m, nil
	}
	return m, nil
}

// attentionEvents reports the banners for runs that just entered a state needing
// the user — finished, failed, or waiting. prev is nil on the first load, which
// seeds silently so opening the monitor never rings for runs already in flight.
func attentionEvents(prev map[string]state.Status, runs []state.Run) []string {
	if prev == nil {
		return nil
	}
	var events []string
	for _, run := range runs {
		old, seen := prev[run.ID]
		if !seen || old == run.Status {
			continue
		}
		if banner, ok := attentionBanner(run); ok {
			events = append(events, banner)
		}
	}
	return events
}

// attentionBanner is the notice for a run that reached an attention state, and
// whether its state is one worth announcing.
func attentionBanner(run state.Run) (string, bool) {
	switch run.Status {
	case state.StatusWaiting:
		return "◐ " + run.Name + " needs input", true
	case state.StatusDone:
		return "✔ " + run.Name + " finished", true
	case state.StatusError:
		return "✖ " + run.Name + " failed", true
	}
	return "", false
}

// statusIndex maps each run's ID to its status, for the next load to diff.
func statusIndex(runs []state.Run) map[string]state.Status {
	index := make(map[string]state.Status, len(runs))
	for _, run := range runs {
		index[run.ID] = run.Status
	}
	return index
}

// ringBell emits the terminal bell. It is a seam so tests observe the signal
// without a real terminal; the default writes BEL to stderr, which rings the
// terminal (and flags the tab in many emulators) without disturbing the display.
var ringBell = func() { fmt.Fprint(os.Stderr, "\a") }

// bellCmd rings the terminal bell as a command, off the pure Update path.
func bellCmd() tea.Cmd {
	return func() tea.Msg {
		ringBell()
		return nil
	}
}

// filterKey edits the filter query while the filter input is active.
func (m Model) filterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyEsc:
		m.filter = ""
		m.filtering = false
	case tea.KeyBackspace:
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		m.filter += " "
	case tea.KeyRunes:
		m.filter += string(msg.Runes)
	}
	m.cursor = clamp(m.cursor, len(m.visible()))
	return m, nil
}

// commandKey edits the command being typed after ":", running it on Enter.
func (m Model) commandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.commanding = false
		return m.dispatch(m.command)
	case tea.KeyEsc:
		m.commanding = false
		m.command = ""
	case tea.KeyBackspace:
		if r := []rune(m.command); len(r) > 0 {
			m.command = string(r[:len(r)-1])
		}
	case tea.KeySpace:
		m.command += " "
	case tea.KeyRunes:
		m.command += string(msg.Runes)
	}
	return m, nil
}

// dispatch runs a typed command against the selected run. Unknown or malformed
// commands report a message instead of acting.
func (m Model) dispatch(command string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(command)
	m.command = ""
	if len(fields) == 0 {
		return m, nil
	}
	switch fields[0] {
	case "rename":
		name := strings.TrimSpace(strings.TrimPrefix(command, fields[0]))
		if name == "" || m.cursor >= len(m.visible()) {
			m.commandMsg = "usage: rename <new name>"
			return m, nil
		}
		id := m.visible()[m.cursor].ID
		m.commandMsg = "renamed to " + name
		return m, renameCmd(id, name, m.discover)
	case "discovery":
		m.discover = !m.discover
		if m.discover {
			m.commandMsg = "discovery on — scanning running agents"
		} else {
			m.commandMsg = "discovery off"
		}
		return m, m.reload()
	case "global":
		m.global = true
		m.commandMsg = "scope: global — every directory"
		m.cursor = clamp(m.cursor, len(m.visible()))
		return m, nil
	case "local":
		m.global = false
		m.commandMsg = "scope: local — this directory"
		m.cursor = clamp(m.cursor, len(m.visible()))
		return m, nil
	case "kill":
		// Typing the whole command is deliberate enough to kill immediately; the
		// k key, one keystroke away from the cursor keys, confirms first instead.
		pid, name, reason, ok := m.killTarget()
		if !ok {
			m.commandMsg = reason
			return m, nil
		}
		m.commandMsg = "killing " + name
		return m, killCmd(pid, m.discover)
	default:
		m.commandMsg = "unknown command: " + fields[0]
		return m, nil
	}
}

// killTarget validates the selected run for killing. It returns the process to
// signal and its name, or a reason the run cannot be killed.
func (m Model) killTarget() (pid int, name, reason string, ok bool) {
	if m.cursor >= len(m.visible()) {
		return 0, "", "no agent selected", false
	}
	run := m.visible()[m.cursor]
	switch {
	case run.Status != state.StatusRunning:
		return 0, "", "only running agents can be killed", false
	case run.PID == 0:
		return 0, "", "can't kill this agent — no process (a reflected session?)", false
	}
	return run.PID, run.Name, "", true
}

// armKill readies a kill of the selected agent, to be confirmed with y. An
// invalid selection reports why instead of arming.
func (m Model) armKill() Model {
	pid, name, reason, ok := m.killTarget()
	if !ok {
		m.commandMsg = reason
		return m
	}
	m.confirmKill = true
	m.killPID = pid
	m.killName = name
	m.commandMsg = ""
	return m
}

// confirmKillKey resolves the k-key confirmation: y (or enter) signals the armed
// process; anything else cancels.
func (m Model) confirmKillKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.confirmKill = false
	switch msg.String() {
	case "y", "enter":
		m.commandMsg = "killing " + m.killName
		return m, killCmd(m.killPID, m.discover)
	default:
		m.commandMsg = "kill canceled"
		return m, nil
	}
}

// renameCmd records a custom name for id and reloads so the change shows at once.
func renameCmd(id, name string, discover bool) tea.Cmd {
	return func() tea.Msg {
		_ = state.WriteName(state.Dir(), id, name)
		return loadRuns(discover)
	}
}

// killProcess signals a process to terminate. It is a seam so the kill flow can
// be tested without terminating unrelated processes.
var killProcess = func(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// killCmd terminates the agent process with the given PID, then reloads so its
// new terminal state (recorded by its launcher) shows.
func killCmd(pid int, discover bool) tea.Cmd {
	return func() tea.Msg {
		_ = killProcess(pid)
		return loadRuns(discover)
	}
}

// visible is the run list the table shows: scoped to the launch directory unless
// global, only running agents unless showAll is set, and narrowed by the active
// text filter.
func (m Model) visible() []state.Run {
	out := make([]state.Run, 0, len(m.runs))
	for _, run := range m.runs {
		if !m.inScope(run) {
			continue
		}
		if !m.showAll && !isActive(run.Status) {
			continue
		}
		if m.filter != "" && !matchesFilter(run, m.filter) {
			continue
		}
		out = append(out, run)
	}
	return out
}

// isActive reports whether a run is still in play — working or waiting on the
// user — so the default view keeps both and hides only finished runs.
func isActive(s state.Status) bool {
	return s == state.StatusRunning || s == state.StatusWaiting
}

// inScope reports whether run belongs to the active scope. Global shows every
// run; local keeps runs whose directory is the root or below it. A run with no
// directory (an older run, or a producer that recorded none) is local-invisible
// and appears only in global.
func (m Model) inScope(run state.Run) bool {
	if m.global || m.root == "" {
		return true
	}
	return withinDir(run.Dir, m.root)
}

// withinDir reports whether dir is root or a directory beneath it.
func withinDir(dir, root string) bool {
	if dir == "" {
		return false
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// matchesFilter reports whether run satisfies query. A leading `-n ` or `-m `
// scopes the match to the name or model; otherwise either field may match.
func matchesFilter(run state.Run, query string) bool {
	field, term := parseFilter(query)
	term = strings.ToLower(term)
	if term == "" {
		return true
	}
	name := strings.Contains(strings.ToLower(run.Name), term)
	model := strings.Contains(strings.ToLower(run.Model), term)
	switch field {
	case "name":
		return name
	case "model":
		return model
	default:
		return name || model
	}
}

// parseFilter splits a query into an optional field scope and its term.
func parseFilter(query string) (field, term string) {
	switch {
	case strings.HasPrefix(query, "-n "):
		return "name", query[3:]
	case strings.HasPrefix(query, "-m "):
		return "model", query[3:]
	default:
		return "", query
	}
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
	waiting int
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
		case state.StatusWaiting:
			m.waiting++
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
	colorAccent   = brand.Accent          // aqua — the Akuaku accent
	colorStone    = lipgloss.Color("240") // border gray
	colorRunning  = lipgloss.Color("42")  // green — live
	colorWaiting  = lipgloss.Color("214") // amber — needs the user
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
	// outerPadX/Y frame the whole dashboard so it never sits flush to the edges.
	outerPadX = 2
	outerPadY = 1
	// minBoxHeight keeps the table box from collapsing on a very short terminal.
	minBoxHeight = 5
)

// View renders the current frame: the single-run detail when a run is selected
// and open, otherwise the full-width dashboard.
func (m Model) View() string {
	if m.detail && len(m.visible()) > 0 {
		return m.detailView()
	}
	return m.listView()
}

// listView renders the dashboard to fill the terminal: an overview strip, the
// agent table stretched to the remaining height, and a keybinding footer, all
// framed by outer padding.
func (m Model) listView() string {
	runs := m.visible()

	width := m.width - 2*outerPadX
	if width <= 0 {
		width = 80
	}

	// The overview always summarizes every run, so the counts stay stable while
	// the table below shows just the current view (running-only, all, filtered).
	header := m.header(width, computeMetrics(m.runs))
	footer := m.footer()

	// Grow the table box to fill the height left over below the header and
	// above the footer. A height of 0 means "size to content" (e.g. in tests
	// before the first window-size message).
	boxHeight := 0
	if m.height > 0 {
		boxHeight = m.height - 2*outerPadY - lines(header) - lines(footer)
		if boxHeight < minBoxHeight {
			boxHeight = minBoxHeight
		}
	}

	return appStyle.Render(strings.Join([]string{header, m.table(width, runs, boxHeight), footer}, "\n"))
}

// lines counts the rows in a rendered block.
func lines(s string) int { return strings.Count(s, "\n") + 1 }

// footer shows the keybindings, or the filter/command input while it is edited.
func (m Model) footer() string {
	if m.filtering {
		return headerStyle.Render("filter: ") + m.filter + headerStyle.Render("▏  (enter apply · esc clear)")
	}
	if m.commanding {
		return headerStyle.Render(":") + m.command + headerStyle.Render("▏  (enter run · esc cancel)")
	}
	if m.confirmKill {
		return headerStyle.Render("kill ") + m.killName + headerStyle.Render(" ?  (y confirm · n cancel)")
	}
	view := "a all"
	if m.showAll {
		view = "a running"
	}
	scope := "scope:local"
	if m.global {
		scope = "scope:global"
	}
	hint := "↑/↓ move · enter open · k kill · / filter · : cmd · " + view + " · " + scope + " · q quit"
	if note := m.note(); note != "" {
		return note + headerStyle.Render("   ·   "+hint)
	}
	return headerStyle.Render(hint)
}

// note is the attention banner, last command result, or active filter, shown
// beside the hints. The banner leads and is colored, so a finished or waiting
// agent stands out until the next keypress dismisses it.
func (m Model) note() string {
	switch {
	case m.alert != "":
		return lipgloss.NewStyle().Bold(true).Foreground(colorWaiting).Render(m.alert)
	case m.commandMsg != "":
		return headerStyle.Render(m.commandMsg)
	case m.filter != "":
		return headerStyle.Render("filter: ") + m.filter
	default:
		return ""
	}
}

// header is the k9s-style top strip: run stats on the left and the Akuaku logo
// right-aligned. The logo is the brand mark that replaces the emoji.
func (m Model) header(width int, mt metrics) string {
	logo := brand.Logo()
	logoW := 0
	for _, art := range logo {
		if w := lipgloss.Width(art); w > logoW {
			logoW = w
		}
	}

	left := []string{
		fmt.Sprintf("running %d · waiting %d · done %d · err %d", mt.running, mt.waiting, mt.done, mt.errored),
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

// table renders the bordered agent list, filling width and (when boxHeight > 0)
// height, with the selected row marked and each row colored by status. The name
// column is capped so a wide terminal leaves trailing space rather than
// stretching the name into a gap.
func (m Model) table(width int, runs []state.Run, boxHeight int) string {
	innerW := width - 4
	if innerW < minInner {
		innerW = minInner
	}
	nameW := innerW - fixedCols
	if nameW < minNameW {
		nameW = minNameW
	}
	if nameW > maxNameW {
		nameW = maxNameW
	}

	var b strings.Builder
	live := headerStyle.Render("● live")
	b.WriteString(padRight(fmt.Sprintf("Agents (%d)", len(runs)), innerW-lipgloss.Width(live)))
	b.WriteString(live)

	if len(runs) == 0 {
		b.WriteString("\n" + m.emptyMessage())
	} else {
		b.WriteByte('\n')
		b.WriteString(headerStyle.Render(formatRow(" ", " ", "NAME", "BACKEND", "MODEL", "DUR", "TOKENS", "COST", nameW)))
		for i, run := range runs {
			marker := " "
			if i == m.cursor {
				marker = ">"
			}
			row := formatRow(marker, m.glyphFor(run.Status), run.Name, run.Backend, run.Model,
				formatDuration(duration(run, m.now)), formatTokens(run), formatCost(run), nameW)
			b.WriteByte('\n')
			b.WriteString(rowStyle(run.Status, i == m.cursor).Render(row))
		}
	}

	box := boxStyle.Width(width - 2)
	if boxHeight > 2 {
		box = box.Height(boxHeight - 2)
	}
	return box.Render(b.String())
}

// emptyMessage explains why the table is empty: no filter match, no runs at all,
// or (the default) runs exist but none are running.
func (m Model) emptyMessage() string {
	switch {
	case m.filter != "":
		return "no agents match the filter — esc to clear"
	case len(m.runs) == 0:
		return "no agents yet — launch one with `akuaku run`"
	default:
		return "no running agents — press a to show all"
	}
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
	case state.StatusWaiting:
		style = style.Foreground(colorWaiting)
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

// spinnerFrames animate a running row so the dashboard reads as alive between
// state refreshes. Each is one display column wide, like the static glyphs.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// glyphFor is the leading marker for a run: a spinner frame while running,
// otherwise the run's static status glyph.
func (m Model) glyphFor(s state.Status) string {
	if s == state.StatusRunning {
		return spinnerFrames[m.frame%len(spinnerFrames)]
	}
	return statusGlyph(s)
}

// statusGlyph is the leading status marker for a run.
func statusGlyph(s state.Status) string {
	switch s {
	case state.StatusRunning:
		return "●"
	case state.StatusWaiting:
		return "◐"
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
	run := m.visible()[m.cursor]
	you := lipgloss.NewStyle().Bold(true).Foreground(colorRunning)
	agent := lipgloss.NewStyle().Bold(true).Foreground(colorAccent)

	var b strings.Builder
	b.WriteString(titleStyle.Render("akuaku") + "  " + lipgloss.NewStyle().Bold(true).Render(run.Name))
	b.WriteByte('\n')
	fmt.Fprintf(&b, "%s · %s · %s tok · %s\n\n",
		headerStyle.Render(run.Backend), run.Status, formatTokens(run), formatCost(run))

	// The exchange, framed as a conversation.
	b.WriteString(you.Render("You"))
	b.WriteByte('\n')
	b.WriteString(indent(orDim(run.Task, "(no prompt recorded)")))
	b.WriteString("\n\n")

	b.WriteString(agent.Render("🗿 " + run.Backend))
	b.WriteByte('\n')
	switch {
	case run.Status == state.StatusError && run.Error != "":
		b.WriteString(indent(run.Error))
	case run.Output != "":
		b.WriteString(indent(run.Output))
	default:
		b.WriteString(indent(headerStyle.Render("(no output captured)")))
	}
	b.WriteString("\n\n")

	if m.commanding {
		b.WriteString(headerStyle.Render(":") + m.command + headerStyle.Render("▏  (enter run · esc cancel)"))
	} else if m.commandMsg != "" {
		b.WriteString(headerStyle.Render(m.commandMsg + "   ·   : rename · : kill · esc back · q quit"))
	} else {
		b.WriteString(headerStyle.Render(": rename · : kill · esc back · q quit"))
	}
	b.WriteByte('\n')
	return appStyle.Render(b.String())
}

// indent prefixes every line of s with two spaces so conversation turns are set
// off from their speaker label.
func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// orDim returns s, or a faint fallback when s is empty.
func orDim(s, fallback string) string {
	if s == "" {
		return headerStyle.Render(fallback)
	}
	return s
}
