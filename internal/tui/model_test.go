package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akuaku-ai/akuaku/internal/state"
)

func TestNew_StartsEmpty(t *testing.T) {
	m := New()
	if len(m.runs) != 0 {
		t.Errorf("New() runs = %d, want 0", len(m.runs))
	}
}

func TestInit_ReturnsCommand(t *testing.T) {
	if New().Init() == nil {
		t.Fatal("Init() = nil, want a command")
	}
}

func TestUpdate_QuitKeys(t *testing.T) {
	keys := map[string]tea.KeyMsg{
		"q":      {Type: tea.KeyRunes, Runes: []rune{'q'}},
		"ctrl+c": {Type: tea.KeyCtrlC},
	}
	for name, key := range keys {
		t.Run(name, func(t *testing.T) {
			_, cmd := New().Update(key)
			if cmd == nil {
				t.Fatal("expected a quit command, got nil")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("command did not produce tea.QuitMsg")
			}
		})
	}
}

func TestUpdate_OtherKeyIsIgnored(t *testing.T) {
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	updated, cmd := New().Update(key)
	if cmd != nil {
		t.Fatal("expected nil cmd for a non-quit key")
	}
	if len(updated.(Model).runs) != 0 {
		t.Fatal("model changed on a non-quit key")
	}
}

func TestUpdate_TickStoresNowAndReloads(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	updated, cmd := New().Update(newTickMsg(now))
	if !updated.(Model).now.Equal(now) {
		t.Errorf("now not stored: %v", updated.(Model).now)
	}
	if cmd == nil {
		t.Fatal("expected a reload/reschedule command")
	}
}

func TestUpdate_RunsMessageStoresRunsAndError(t *testing.T) {
	runs := []state.Run{{ID: "a"}}
	updated, cmd := New().Update(runsMsg{runs: runs, err: nil})
	m := updated.(Model)
	if len(m.runs) != 1 || m.runs[0].ID != "a" {
		t.Errorf("runs not stored: %+v", m.runs)
	}
	if cmd != nil {
		t.Error("runsMsg should not schedule a command")
	}
}

func TestUpdate_UnknownMessageIsIgnored(t *testing.T) {
	type otherMsg struct{}
	updated, cmd := New().Update(otherMsg{})
	if cmd != nil {
		t.Fatal("expected nil cmd for an unknown message")
	}
	if len(updated.(Model).runs) != 0 {
		t.Fatal("model changed on an unknown message")
	}
}

func TestLoadRuns_ReadsStateDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AKUAKU_STATE_DIR", dir)
	if err := state.Write(dir, state.Run{ID: "claude-1-a", Backend: "claude", Status: state.StatusDone, StartedAt: time.Unix(1, 0).UTC()}); err != nil {
		t.Fatal(err)
	}

	msg, ok := loadRuns().(runsMsg)
	if !ok {
		t.Fatalf("loadRuns returned %T, want runsMsg", loadRuns())
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if len(msg.runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(msg.runs))
	}
}

func TestLoadRuns_ReportsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AKUAKU_STATE_DIR", file)

	if msg := loadRuns().(runsMsg); msg.err == nil {
		t.Fatal("expected an error when the state dir is a file")
	}
}

func TestDuration_LiveWhileRunning(t *testing.T) {
	run := state.Run{Status: state.StatusRunning, StartedAt: time.Unix(0, 0)}
	if got := duration(run, time.Unix(12, 0)); got != 12*time.Second {
		t.Errorf("duration = %v, want 12s", got)
	}
}

func TestDuration_FrozenWhenTerminal(t *testing.T) {
	ended := time.Unix(30, 0)
	run := state.Run{Status: state.StatusDone, StartedAt: time.Unix(0, 0), EndedAt: &ended}
	if got := duration(run, time.Unix(999, 0)); got != 30*time.Second {
		t.Errorf("duration = %v, want 30s (frozen)", got)
	}
}

func TestDuration_TerminalWithoutEndFallsBackToNow(t *testing.T) {
	run := state.Run{Status: state.StatusDone, StartedAt: time.Unix(0, 0)}
	if got := duration(run, time.Unix(7, 0)); got != 7*time.Second {
		t.Errorf("duration = %v, want 7s", got)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		5 * time.Second:               "0:05",
		65 * time.Second:              "1:05",
		2*time.Minute + 3*time.Second: "2:03",
		-1 * time.Second:              "0:00",
	}
	for d, want := range cases {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestComputeMetrics_AggregatesByStatus(t *testing.T) {
	runs := []state.Run{
		{Status: state.StatusRunning, Tokens: 10, Cost: 0.1},
		{Status: state.StatusDone, Tokens: 20, Cost: 0.2},
		{Status: state.StatusError, Tokens: 5, Cost: 0.0},
		{Status: state.StatusDone, Tokens: 1, Cost: 0.05},
	}
	m := computeMetrics(runs)
	if m.running != 1 || m.done != 2 || m.errored != 1 {
		t.Errorf("counts wrong: %+v", m)
	}
	if m.tokens != 36 {
		t.Errorf("tokens = %d, want 36", m.tokens)
	}
	if m.cost < 0.349 || m.cost > 0.351 {
		t.Errorf("cost = %v, want ~0.35", m.cost)
	}
}

func TestView_RendersRunsAndMetrics(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	runs := []state.Run{
		{ID: "1", Name: "refactor", Backend: "claude", Status: state.StatusRunning, StartedAt: time.Unix(90, 0).UTC(), Tokens: 1200, Cost: 0.04},
	}
	out := Model{runs: runs, now: now}.View()

	for _, want := range []string{"Akuaku", "refactor", "claude", "running", "1200", "0.04", "running:"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q, got:\n%s", want, out)
		}
	}
}

func TestView_ReflectedRunShowsDashForUsage(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	runs := []state.Run{
		{ID: "s1", Name: "external session", Backend: "claude", Status: state.StatusRunning,
			Source: "hook", StartedAt: time.Unix(90, 0).UTC()},
	}
	out := Model{runs: runs, now: now}.View()

	if !strings.Contains(out, "external session") {
		t.Errorf("reflected run not shown, got:\n%s", out)
	}
	if !strings.Contains(out, "—") {
		t.Errorf("reflected usage should render as a dash, got:\n%s", out)
	}
	// A hook run carries no usage, so a bare zero count or a dollar figure would
	// misrepresent it as measured.
	line := runLine(out, "external session")
	if strings.Contains(line, "$") {
		t.Errorf("reflected run should not show a cost figure: %q", line)
	}
}

func TestFormatTokens(t *testing.T) {
	if got := formatTokens(state.Run{Tokens: 1200}); got != "1200" {
		t.Errorf("formatTokens = %q, want 1200", got)
	}
	if got := formatTokens(state.Run{Source: "hook", Tokens: 1200}); got != "—" {
		t.Errorf("hook formatTokens = %q, want a dash", got)
	}
}

func TestFormatCost(t *testing.T) {
	if got := formatCost(state.Run{Cost: 0.04}); got != "$0.04" {
		t.Errorf("formatCost = %q, want $0.04", got)
	}
	if got := formatCost(state.Run{Source: "hook", Cost: 0.04}); got != "—" {
		t.Errorf("hook formatCost = %q, want a dash", got)
	}
}

// runLine returns the first View line containing name, for asserting on a
// single run's row.
func runLine(view, name string) string {
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, name) {
			return l
		}
	}
	return ""
}

func TestView_ShowsEmptyState(t *testing.T) {
	out := New().View()
	if !strings.Contains(out, "no agents") {
		t.Errorf("empty View() should hint at no agents, got:\n%s", out)
	}
}

func TestView_ShowsError(t *testing.T) {
	out := Model{err: errBoom}.View()
	if !strings.Contains(out, "boom") {
		t.Errorf("View() should show the load error, got:\n%s", out)
	}
}

func TestNewTickMsg_WrapsTime(t *testing.T) {
	now := time.Unix(42, 0)
	if got := newTickMsg(now); got != tickMsg(now) {
		t.Fatalf("newTickMsg = %v, want %v", got, tickMsg(now))
	}
}

func TestTickCmd_ReturnsCommand(t *testing.T) {
	if tickCmd() == nil {
		t.Fatal("tickCmd() = nil, want a command")
	}
}

// threeRuns is a small fixture for cursor and detail tests.
func threeRuns() []state.Run {
	return []state.Run{
		{ID: "a", Name: "one", Backend: "claude", Status: state.StatusDone},
		{ID: "b", Name: "two", Backend: "codex", Status: state.StatusDone},
		{ID: "c", Name: "three", Backend: "ollama", Status: state.StatusDone},
	}
}

func TestUpdate_DownMovesCursorAndStopsAtEnd(t *testing.T) {
	m := Model{runs: threeRuns()}
	for _, want := range []int{1, 2, 2} { // clamps at the last row
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(Model)
		if m.cursor != want {
			t.Errorf("cursor = %d, want %d", m.cursor, want)
		}
	}
}

func TestUpdate_UpMovesCursorAndStopsAtTop(t *testing.T) {
	m := Model{runs: threeRuns(), cursor: 2}
	for _, want := range []int{1, 0, 0} { // clamps at the first row
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m = next.(Model)
		if m.cursor != want {
			t.Errorf("cursor = %d, want %d", m.cursor, want)
		}
	}
}

func TestUpdate_EnterOpensDetailWhenRunsExist(t *testing.T) {
	next, _ := Model{runs: threeRuns()}.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !next.(Model).detail {
		t.Error("enter should open the detail view")
	}
}

func TestUpdate_EnterWithoutRunsStaysInList(t *testing.T) {
	next, _ := New().Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(Model).detail {
		t.Error("enter with no runs should not open detail")
	}
}

func TestUpdate_EscClosesDetail(t *testing.T) {
	m := Model{runs: threeRuns(), detail: true}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if next.(Model).detail {
		t.Error("esc should close the detail view")
	}
}

func TestUpdate_RunsMessageClampsCursor(t *testing.T) {
	// The selected row disappears when runs shrink; the cursor must stay valid.
	m := Model{runs: threeRuns(), cursor: 2}
	next, _ := m.Update(runsMsg{runs: []state.Run{{ID: "a", Name: "one"}}})
	if got := next.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d, want 0 after shrink", got)
	}
	// Empty runs must not leave a negative cursor.
	next, _ = m.Update(runsMsg{runs: nil})
	if got := next.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d, want 0 when empty", got)
	}
}

func TestView_ListMarksSelectedRow(t *testing.T) {
	out := Model{runs: threeRuns(), cursor: 1}.View()
	if line := runLine(out, "two"); !strings.HasPrefix(strings.TrimLeft(line, " "), ">") {
		t.Errorf("selected row should be marked, got %q", line)
	}
	if line := runLine(out, "one"); strings.Contains(line, ">") {
		t.Errorf("unselected row should not be marked, got %q", line)
	}
}

func TestView_DetailShowsOutput(t *testing.T) {
	runs := []state.Run{{Name: "review", Backend: "claude", Status: state.StatusDone,
		Task: "2 tips", Tokens: 124, Cost: 0.11, Output: "1. small PRs\n2. tests first"}}
	out := Model{runs: runs, detail: true}.View()

	for _, want := range []string{"review", "claude", "2 tips", "124", "0.11", "1. small PRs", "esc"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail view missing %q, got:\n%s", want, out)
		}
	}
}

func TestView_DetailShowsErrorForFailedRun(t *testing.T) {
	runs := []state.Run{{Name: "boom", Backend: "claude", Status: state.StatusError, Error: "model not found"}}
	out := Model{runs: runs, detail: true}.View()
	if !strings.Contains(out, "model not found") {
		t.Errorf("detail view should show the error, got:\n%s", out)
	}
}

func TestView_DetailReflectedRunHasDashesAndNoOutput(t *testing.T) {
	// A hook run has no task, no output, and unknown usage.
	runs := []state.Run{{Name: "external", Backend: "claude", Status: state.StatusRunning, Source: "hook"}}
	out := Model{runs: runs, detail: true}.View()
	if !strings.Contains(out, "—") {
		t.Errorf("reflected usage should render as a dash, got:\n%s", out)
	}
	if !strings.Contains(out, "no output") {
		t.Errorf("a run without output should say so, got:\n%s", out)
	}
}

func TestView_DetailIgnoredWhenNoRuns(t *testing.T) {
	// detail mode with an empty list falls back to the list view safely.
	out := Model{detail: true}.View()
	if !strings.Contains(out, "no agents") {
		t.Errorf("empty detail should fall back to the list, got:\n%s", out)
	}
}

var errBoom = boomError("boom")

type boomError string

func (e boomError) Error() string { return string(e) }
