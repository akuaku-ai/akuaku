package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/akuaku-ai/akuaku/internal/state"
)

func TestNew_StartsEmpty(t *testing.T) {
	m := New()
	if len(m.runs) != 0 {
		t.Errorf("New() runs = %d, want 0", len(m.runs))
	}
}

func TestNew_DiscoveryOnByDefault(t *testing.T) {
	if !New().discover {
		t.Error("discovery should be on by default so running sessions appear without setup")
	}
}

func TestNew_ScopesToLaunchDirectory(t *testing.T) {
	old := getwd
	defer func() { getwd = old }()
	getwd = func() (string, error) { return "/home/u/proj", nil }

	m := New()
	if m.root != "/home/u/proj" {
		t.Errorf("root = %q, want the launch directory", m.root)
	}
	if m.global {
		t.Error("scope should default to local")
	}
}

func TestWithinDir(t *testing.T) {
	cases := []struct {
		dir, root string
		want      bool
	}{
		{"/root/proj", "/root/proj", true},     // the root itself
		{"/root/proj/sub", "/root/proj", true}, // a directory below the root
		{"/root", "/root/proj", false},         // the parent of the root
		{"/root/other", "/root/proj", false},   // a sibling
		{"", "/root/proj", false},              // no directory recorded
		{"relative", "/root/proj", false},      // not resolvable against an absolute root
	}
	for _, c := range cases {
		if got := withinDir(c.dir, c.root); got != c.want {
			t.Errorf("withinDir(%q, %q) = %v, want %v", c.dir, c.root, got, c.want)
		}
	}
}

func TestInScope_GlobalAndEmptyRootShowEverything(t *testing.T) {
	elsewhere := state.Run{Dir: "/other"}
	if !(Model{global: true, root: "/root"}).inScope(elsewhere) {
		t.Error("global scope should show every directory")
	}
	if !(Model{root: ""}).inScope(elsewhere) {
		t.Error("an unknown root should not hide runs")
	}
	if (Model{root: "/root"}).inScope(elsewhere) {
		t.Error("local scope should hide a run from another directory")
	}
}

func TestVisible_LocalScopeKeepsOnlyRootAndBelow(t *testing.T) {
	runs := []state.Run{
		{ID: "here", Status: state.StatusRunning, Dir: "/root/proj"},
		{ID: "below", Status: state.StatusRunning, Dir: "/root/proj/sub"},
		{ID: "away", Status: state.StatusRunning, Dir: "/root/other"},
		{ID: "nodir", Status: state.StatusRunning, Dir: ""},
	}

	local := Model{runs: runs, root: "/root/proj"}.visible()
	if len(local) != 2 {
		t.Fatalf("local scope should show 2 runs (root + below), got %d: %+v", len(local), local)
	}

	global := Model{runs: runs, root: "/root/proj", global: true}.visible()
	if len(global) != 4 {
		t.Fatalf("global scope should show all 4 runs, got %d", len(global))
	}
}

func TestDispatch_ScopeCommandsToggleGlobal(t *testing.T) {
	toGlobal, _ := Model{runs: threeRuns()}.dispatch("global")
	if !toGlobal.(Model).global || !strings.Contains(toGlobal.(Model).commandMsg, "global") {
		t.Errorf("`:global` did not switch to global: %+v", toGlobal.(Model).commandMsg)
	}

	toLocal, _ := toGlobal.(Model).dispatch("local")
	if toLocal.(Model).global || !strings.Contains(toLocal.(Model).commandMsg, "local") {
		t.Errorf("`:local` did not switch back to local: %+v", toLocal.(Model).commandMsg)
	}
}

func TestAttentionEvents_FirstLoadIsSilent(t *testing.T) {
	if e := attentionEvents(nil, []state.Run{{ID: "a", Status: state.StatusDone}}); e != nil {
		t.Errorf("opening the monitor must not ring for runs already in flight, got %v", e)
	}
}

func TestAttentionEvents_EmitsOnTransitionToAttentionState(t *testing.T) {
	prev := map[string]state.Status{
		"a": state.StatusRunning, "b": state.StatusRunning, "c": state.StatusRunning,
		"d": state.StatusDone, "f": state.StatusRunning, "g": state.StatusWaiting,
	}
	runs := []state.Run{
		{ID: "a", Name: "A", Status: state.StatusDone},    // running → done: announce
		{ID: "b", Name: "B", Status: state.StatusWaiting}, // running → waiting: announce
		{ID: "c", Name: "C", Status: state.StatusRunning}, // unchanged: silent
		{ID: "d", Name: "D", Status: state.StatusDone},    // unchanged: silent
		{ID: "e", Name: "E", Status: state.StatusDone},    // new run: seed, silent
		{ID: "f", Name: "F", Status: state.StatusError},   // running → error: announce
		{ID: "g", Name: "G", Status: state.StatusRunning}, // waiting → running: silent (not attention)
	}

	events := attentionEvents(prev, runs)
	if len(events) != 3 {
		t.Fatalf("want 3 events (done, waiting, error), got %d: %v", len(events), events)
	}
	joined := strings.Join(events, " | ")
	for _, want := range []string{"A finished", "B needs input", "F failed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("events missing %q: %v", want, events)
		}
	}
}

func TestRunsMsg_SeedsThenRingsOnTransition(t *testing.T) {
	orig := ringBell
	rung := 0
	ringBell = func() { rung++ }
	defer func() { ringBell = orig }()

	first, cmd1 := Model{}.Update(runsMsg{runs: []state.Run{{ID: "a", Name: "task", Status: state.StatusRunning}}})
	if first.(Model).alert != "" || cmd1 != nil {
		t.Fatalf("first load should seed silently: alert=%q cmd=%v", first.(Model).alert, cmd1)
	}

	second, cmd2 := first.(Model).Update(runsMsg{runs: []state.Run{{ID: "a", Name: "task", Status: state.StatusDone}}})
	if !strings.Contains(second.(Model).alert, "finished") {
		t.Errorf("alert = %q, want a finished banner", second.(Model).alert)
	}
	if cmd2 == nil {
		t.Fatal("a transition should ring the bell")
	}
	if msg := cmd2(); msg != nil {
		t.Errorf("bell cmd should return nil msg, got %v", msg)
	}
	if rung != 1 {
		t.Errorf("bell rung %d times, want 1", rung)
	}
}

func TestRingBell_DefaultDoesNotPanic(_ *testing.T) {
	ringBell() // exercises the default stderr write for coverage
}

func TestView_ShowsAttentionBanner(t *testing.T) {
	out := Model{runs: threeRuns(), width: 100, alert: "✔ task finished"}.View()
	if !strings.Contains(out, "task finished") {
		t.Errorf("attention banner not shown, got:\n%s", out)
	}
}

func TestUpdate_KeypressDismissesAlert(t *testing.T) {
	next, _ := Model{runs: threeRuns(), alert: "✔ x finished"}.Update(tea.KeyMsg{Type: tea.KeyDown})
	if next.(Model).alert != "" {
		t.Errorf("a keypress should dismiss the banner, got %q", next.(Model).alert)
	}
}

func TestComputeMetrics_CountsWaiting(t *testing.T) {
	m := computeMetrics([]state.Run{
		{Status: state.StatusWaiting},
		{Status: state.StatusWaiting},
		{Status: state.StatusRunning},
	})
	if m.waiting != 2 || m.running != 1 {
		t.Errorf("waiting=%d running=%d, want 2/1", m.waiting, m.running)
	}
}

func TestVisible_DefaultShowsWaitingAndRunning(t *testing.T) {
	runs := []state.Run{
		{ID: "r", Status: state.StatusRunning},
		{ID: "w", Status: state.StatusWaiting},
		{ID: "d", Status: state.StatusDone},
	}
	if got := (Model{runs: runs}).visible(); len(got) != 2 {
		t.Fatalf("default view should show running + waiting (needs attention), got %d: %+v", len(got), got)
	}
}

func TestView_WaitingRunShowsGlyphAndCount(t *testing.T) {
	runs := []state.Run{{ID: "w", Name: "review", Status: state.StatusWaiting, StartedAt: time.Unix(90, 0)}}
	out := Model{runs: runs, now: time.Unix(100, 0), width: 100}.View()
	if !strings.Contains(out, "◐") {
		t.Errorf("waiting run should show the attention glyph, got:\n%s", out)
	}
	if !strings.Contains(out, "waiting 1") {
		t.Errorf("header should count waiting agents, got:\n%s", out)
	}
}

func TestView_FooterShowsScope(t *testing.T) {
	local := Model{runs: threeRuns(), width: 100}.View()
	if !strings.Contains(local, "scope:local") {
		t.Errorf("footer should show local scope, got:\n%s", local)
	}
	global := Model{runs: threeRuns(), width: 100, global: true}.View()
	if !strings.Contains(global, "scope:global") {
		t.Errorf("footer should show global scope, got:\n%s", global)
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

	msg, ok := loadRuns(false).(runsMsg)
	if !ok {
		t.Fatalf("loadRuns returned %T, want runsMsg", loadRuns(false))
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

	if msg := loadRuns(false).(runsMsg); msg.err == nil {
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
		{ID: "1", Name: "refactor", Backend: "claude", Model: "opus", Status: state.StatusRunning, StartedAt: time.Unix(90, 0).UTC(), Tokens: 1200, Cost: 0.04},
	}
	out := Model{runs: runs, now: now, width: 100}.View()

	for _, want := range []string{"█", "refactor", "claude", "opus", "1200", "0.04", "running 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q, got:\n%s", want, out)
		}
	}
}

func TestView_ShowsLogoAndStats(t *testing.T) {
	out := Model{runs: threeRuns(), width: 100}.View()
	// The brand wordmark, drawn in block characters.
	if !strings.Contains(out, "█") {
		t.Errorf("header should show the Akuaku wordmark, got:\n%s", out)
	}
	// Stats sit on the left of the k9s-style header.
	for _, want := range []string{"running", "live"} {
		if !strings.Contains(out, want) {
			t.Errorf("header missing %q, got:\n%s", want, out)
		}
	}
}

func TestView_ReflectedRunShowsDashForUsage(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	runs := []state.Run{
		{ID: "s1", Name: "external session", Backend: "claude", Status: state.StatusRunning,
			Source: "hook", StartedAt: time.Unix(90, 0).UTC()},
	}
	out := Model{runs: runs, now: now, width: 100}.View()

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

// threeRuns is a small fixture for cursor and render tests. They are running so
// they appear in the default (running-only) view.
func threeRuns() []state.Run {
	return []state.Run{
		{ID: "a", Name: "one", Backend: "claude", Status: state.StatusRunning},
		{ID: "b", Name: "two", Backend: "codex", Status: state.StatusRunning},
		{ID: "c", Name: "three", Backend: "ollama", Status: state.StatusRunning},
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
	out := Model{runs: threeRuns(), cursor: 1, width: 100}.View()
	if line := runLine(out, "two"); !strings.Contains(line, ">") {
		t.Errorf("selected row should be marked, got %q", line)
	}
	if line := runLine(out, "one"); strings.Contains(line, ">") {
		t.Errorf("unselected row should not be marked, got %q", line)
	}
}

func TestView_DetailShowsOutput(t *testing.T) {
	runs := []state.Run{{Name: "review", Backend: "claude", Status: state.StatusDone,
		Task: "2 tips", Tokens: 124, Cost: 0.11, Output: "1. small PRs\n2. tests first"}}
	out := Model{runs: runs, detail: true, showAll: true}.View()

	for _, want := range []string{"review", "claude", "2 tips", "124", "0.11", "1. small PRs", "esc"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail view missing %q, got:\n%s", want, out)
		}
	}
}

func TestView_DetailShowsErrorForFailedRun(t *testing.T) {
	runs := []state.Run{{Name: "boom", Backend: "claude", Status: state.StatusError, Error: "model not found"}}
	out := Model{runs: runs, detail: true, showAll: true}.View()
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

func TestUpdate_WindowSizeStoresDimensions(t *testing.T) {
	next, _ := New().Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m := next.(Model)
	if m.width != 120 || m.height != 40 {
		t.Errorf("dimensions = %dx%d, want 120x40", m.width, m.height)
	}
}

func TestUpdate_RunsMessageSortsRunningFirstThenNewest(t *testing.T) {
	older, newer := time.Unix(100, 0), time.Unix(200, 0)
	runs := []state.Run{
		{Name: "done-old", Status: state.StatusDone, StartedAt: older},
		{Name: "run-old", Status: state.StatusRunning, StartedAt: older},
		{Name: "done-new", Status: state.StatusDone, StartedAt: newer},
		{Name: "run-new", Status: state.StatusRunning, StartedAt: newer},
	}
	next, _ := New().Update(runsMsg{runs: runs})
	got := next.(Model).runs

	want := []string{"run-new", "run-old", "done-new", "done-old"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("row %d = %q, want %q (full order: %+v)", i, got[i].Name, name, got)
		}
	}
}

func TestPadRight(t *testing.T) {
	if got := padRight("ab", 4); got != "ab  " {
		t.Errorf("padRight pad = %q", got)
	}
	if got := padRight("abcdef", 4); got != "abcd" {
		t.Errorf("padRight truncate = %q", got)
	}
}

func TestPadLeft(t *testing.T) {
	if got := padLeft("ab", 4); got != "  ab" {
		t.Errorf("padLeft pad = %q", got)
	}
	if got := padLeft("abcdef", 4); got != "abcd" {
		t.Errorf("padLeft truncate = %q", got)
	}
}

func TestHumanizeTokens(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 1500: "1.5k", 2_500_000: "2.5M"}
	for n, want := range cases {
		if got := humanizeTokens(n); got != want {
			t.Errorf("humanizeTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestStatusGlyph(t *testing.T) {
	cases := map[state.Status]string{
		state.StatusRunning: "●",
		state.StatusError:   "✖",
		state.StatusDone:    "✔",
	}
	for status, want := range cases {
		if got := statusGlyph(status); got != want {
			t.Errorf("statusGlyph(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestView_OverviewShowsCountsAndTotals(t *testing.T) {
	runs := []state.Run{
		{Name: "a", Status: state.StatusRunning, Tokens: 1500, Cost: 0.10},
		{Name: "b", Status: state.StatusDone, Tokens: 2500, Cost: 0.20},
		{Name: "c", Status: state.StatusError},
	}
	out := Model{runs: runs, width: 100}.View()
	for _, want := range []string{"running 1", "done 1", "err 1", "4.0k", "$0.30"} {
		if !strings.Contains(out, want) {
			t.Errorf("overview missing %q, got:\n%s", want, out)
		}
	}
}

func TestView_FooterShowsKeys(t *testing.T) {
	out := Model{runs: threeRuns(), width: 100}.View()
	for _, key := range []string{"move", "enter", "quit"} {
		if !strings.Contains(out, key) {
			t.Errorf("footer missing %q, got:\n%s", key, out)
		}
	}
}

func TestView_RendersBorderedBox(t *testing.T) {
	out := Model{runs: threeRuns(), width: 100}.View()
	if !strings.Contains(out, "│") {
		t.Errorf("expected a full-width bordered box, got:\n%s", out)
	}
}

func TestView_ColorsRunningDoneAndErrorRows(t *testing.T) {
	// One of each status plus a cursor exercises every row style branch.
	runs := []state.Run{
		{Name: "alive", Status: state.StatusRunning},
		{Name: "finished", Status: state.StatusDone},
		{Name: "broken", Status: state.StatusError},
	}
	out := Model{runs: runs, width: 100, cursor: 1, showAll: true}.View()
	for _, want := range []string{"alive", "finished", "broken"} {
		if !strings.Contains(out, want) {
			t.Errorf("row %q not rendered, got:\n%s", want, out)
		}
	}
}

func TestView_HandlesNarrowWidthWithoutPanic(t *testing.T) {
	if out := (Model{runs: threeRuns(), width: 10}).View(); out == "" {
		t.Error("expected output even at a narrow width")
	}
}

func TestView_FillsWidth(t *testing.T) {
	const w = 200
	out := Model{runs: threeRuns(), width: w}.View()
	maxw := 0
	for _, l := range strings.Split(out, "\n") {
		if lw := lipgloss.Width(l); lw > maxw {
			maxw = lw
		}
	}
	if maxw < w-4 || maxw > w {
		t.Errorf("dashboard should fill the width; widest line = %d, want ~%d", maxw, w)
	}
}

func TestView_FillsHeight(t *testing.T) {
	const h = 30
	out := Model{runs: threeRuns(), width: 100, height: h}.View()
	if n := len(strings.Split(out, "\n")); n < h-1 || n > h+1 {
		t.Errorf("dashboard should fill the height; %d lines, want ~%d", n, h)
	}
}

func TestView_TinyHeightDoesNotCollapse(t *testing.T) {
	if out := (Model{runs: threeRuns(), width: 100, height: 3}).View(); out == "" {
		t.Error("expected output even at a tiny height")
	}
}

func TestView_TruncatesVeryLongName(t *testing.T) {
	long := "this is an extremely long agent name that should be truncated to the column"
	out := Model{runs: []state.Run{{Name: long, Status: state.StatusRunning}}, width: 200}.View()
	if strings.Contains(out, long) {
		t.Error("a name longer than the column should be truncated")
	}
}

func TestView_HasOuterPadding(t *testing.T) {
	lines := strings.Split(Model{runs: threeRuns(), width: 100}.View(), "\n")
	if strings.TrimSpace(lines[0]) != "" {
		t.Errorf("expected a blank top-padding line, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("expected left padding on content lines, got %q", lines[1])
	}
}

func TestUpdate_SlashEntersFilter(t *testing.T) {
	next, _ := Model{runs: threeRuns()}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !next.(Model).filtering {
		t.Error("/ should enter filter mode")
	}
}

func TestUpdate_SlashIgnoredInDetail(t *testing.T) {
	next, _ := Model{runs: threeRuns(), detail: true}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if next.(Model).filtering {
		t.Error("/ should not open the filter while viewing a run's detail")
	}
}

func TestFilterKey_TypingBuildsQueryAndNavKeysDoNotMove(t *testing.T) {
	m := Model{runs: threeRuns(), filtering: true, cursor: 2}
	// A nav key like "j" is typed into the filter, not treated as movement.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.filter != "j" {
		t.Errorf("filter = %q, want %q", m.filter, "j")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if got := next.(Model).filter; got != "j x" {
		t.Errorf("filter = %q, want %q", got, "j x")
	}
}

func TestFilterKey_Backspace(t *testing.T) {
	m := Model{filter: "abc", filtering: true}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := next.(Model).filter; got != "ab" {
		t.Errorf("filter = %q, want %q", got, "ab")
	}
	// Backspace on an empty filter is a no-op.
	empty := Model{filtering: true}
	next, _ = empty.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := next.(Model).filter; got != "" {
		t.Errorf("filter = %q, want empty", got)
	}
}

func TestFilterKey_EnterConfirmsAndEscClears(t *testing.T) {
	confirmed, _ := (Model{filter: "foo", filtering: true}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m := confirmed.(Model); m.filtering || m.filter != "foo" {
		t.Errorf("enter should keep the filter and exit editing: %+v", m)
	}
	cleared, _ := (Model{filter: "foo", filtering: true}).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m := cleared.(Model); m.filtering || m.filter != "" {
		t.Errorf("esc should clear the filter and exit editing: %+v", m)
	}
}

func TestMatchesFilter(t *testing.T) {
	run := state.Run{Name: "refactor auth", Model: "claude-opus"}
	cases := map[string]bool{
		"":            true,
		"AUTH":        true,  // case-insensitive, matches name
		"opus":        true,  // matches model
		"nope":        false, // matches neither
		"-n refactor": true,  // name scope
		"-n opus":     false, // name scope excludes model
		"-m opus":     true,  // model scope
		"-m refactor": false, // model scope excludes name
	}
	for query, want := range cases {
		if got := matchesFilter(run, query); got != want {
			t.Errorf("matchesFilter(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestView_FilterHidesNonMatchingRows(t *testing.T) {
	runs := []state.Run{
		{Name: "refactor auth", Backend: "claude", Model: "opus", Status: state.StatusDone},
		{Name: "write tests", Backend: "codex", Model: "5.3-codex", Status: state.StatusDone},
	}
	out := Model{runs: runs, filter: "refactor", width: 100, showAll: true}.View()
	if !strings.Contains(out, "refactor auth") {
		t.Errorf("matching row should show, got:\n%s", out)
	}
	if strings.Contains(out, "write tests") {
		t.Errorf("non-matching row should be hidden, got:\n%s", out)
	}
}

func TestView_ShowsFilterInputWhileEditing(t *testing.T) {
	out := Model{runs: threeRuns(), filter: "ref", filtering: true, width: 100}.View()
	if !strings.Contains(out, "filter: ") || !strings.Contains(out, "ref") {
		t.Errorf("filter input should be shown, got:\n%s", out)
	}
}

func TestView_ShowsActiveFilterHint(t *testing.T) {
	out := Model{runs: threeRuns(), filter: "one", width: 100}.View()
	if !strings.Contains(out, "filter: ") {
		t.Errorf("an applied filter should be indicated, got:\n%s", out)
	}
}

func TestView_EmptyFilterResultExplains(t *testing.T) {
	out := Model{runs: threeRuns(), filter: "zzzznomatch", width: 100}.View()
	if !strings.Contains(out, "no agents match") {
		t.Errorf("an empty filter result should explain itself, got:\n%s", out)
	}
}

func TestUpdate_ATogglesShowAll(t *testing.T) {
	m := Model{runs: threeRuns()}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !next.(Model).showAll {
		t.Error("a should switch to the full history")
	}
	back, _ := next.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if back.(Model).showAll {
		t.Error("a again should switch back to running-only")
	}
}

func TestUpdate_AIgnoredInDetail(t *testing.T) {
	next, _ := Model{runs: threeRuns(), detail: true}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if next.(Model).showAll {
		t.Error("a should not toggle the view while in the detail pane")
	}
}

func TestVisible_DefaultIsRunningOnly(t *testing.T) {
	runs := []state.Run{
		{Name: "live", Status: state.StatusRunning},
		{Name: "done", Status: state.StatusDone},
		{Name: "failed", Status: state.StatusError},
	}
	running := Model{runs: runs}.visible()
	if len(running) != 1 || running[0].Name != "live" {
		t.Errorf("default view should show only running agents, got %+v", running)
	}
	if all := (Model{runs: runs, showAll: true}).visible(); len(all) != 3 {
		t.Errorf("showAll should reveal every agent, got %d", len(all))
	}
}

func TestView_DefaultHidesNonRunningButOverviewCountsAll(t *testing.T) {
	runs := []state.Run{
		{Name: "live-agent", Status: state.StatusRunning},
		{Name: "old-agent", Status: state.StatusDone},
	}
	out := Model{runs: runs, width: 100}.View()
	if !strings.Contains(out, "live-agent") {
		t.Errorf("running agent should be listed, got:\n%s", out)
	}
	if strings.Contains(out, "old-agent") {
		t.Errorf("non-running agent should be hidden by default, got:\n%s", out)
	}
	// The overview still summarizes everything.
	if !strings.Contains(out, "done 1") {
		t.Errorf("overview should count all agents, got:\n%s", out)
	}
}

func TestView_NoRunningAgentsHint(t *testing.T) {
	out := Model{runs: []state.Run{{Name: "old", Status: state.StatusDone}}, width: 100}.View()
	if !strings.Contains(out, "no running agents") {
		t.Errorf("expected a hint to show all, got:\n%s", out)
	}
}

func TestUpdate_ColonEntersCommandMode(t *testing.T) {
	next, _ := Model{runs: threeRuns()}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	if !next.(Model).commanding {
		t.Error(": should enter command mode")
	}
}

func TestCommandKey_TypesRunsAndCancels(t *testing.T) {
	m := Model{runs: threeRuns(), commanding: true}
	for _, r := range []string{"r", "e", "n", "a", "m", "e"} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
		m = next.(Model)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	m = next.(Model)
	if m.command != "rename z" {
		t.Fatalf("command = %q", m.command)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if next.(Model).command != "rename " {
		t.Errorf("backspace failed: %q", next.(Model).command)
	}
	esc, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if em := esc.(Model); em.commanding || em.command != "" {
		t.Errorf("esc should cancel: %+v", em)
	}
	empty, _ := (Model{commanding: true}).Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if empty.(Model).command != "" {
		t.Errorf("backspace on empty should stay empty, got %q", empty.(Model).command)
	}
}

func TestDispatch_RenameWritesOverlayAndReloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AKUAKU_STATE_DIR", dir)
	if err := state.Write(dir, state.Run{ID: "run-1", Name: "old", Status: state.StatusRunning}); err != nil {
		t.Fatal(err)
	}

	m := Model{runs: []state.Run{{ID: "run-1", Name: "old", Status: state.StatusRunning}}, commanding: true, command: "rename brand new"}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	nm := next.(Model)
	if nm.commanding {
		t.Error("enter should leave command mode")
	}
	if !strings.Contains(nm.commandMsg, "brand new") {
		t.Errorf("commandMsg = %q", nm.commandMsg)
	}
	if cmd == nil {
		t.Fatal("rename should return a reload command")
	}
	msg, ok := cmd().(runsMsg)
	if !ok {
		t.Fatalf("rename cmd returned %T", cmd())
	}
	if len(msg.runs) != 1 || msg.runs[0].Name != "brand new" {
		t.Errorf("reloaded runs did not pick up the rename: %+v", msg.runs)
	}
}

func TestDispatch_RenameWithoutNameShowsUsage(t *testing.T) {
	next, cmd := (Model{runs: threeRuns(), commanding: true, command: "rename"}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("a nameless rename should not act")
	}
	if !strings.Contains(next.(Model).commandMsg, "usage") {
		t.Errorf("commandMsg = %q", next.(Model).commandMsg)
	}
}

func TestDispatch_UnknownAndEmptyCommand(t *testing.T) {
	unknown, _ := (Model{runs: threeRuns(), commanding: true, command: "frobnicate"}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(unknown.(Model).commandMsg, "unknown command") {
		t.Errorf("commandMsg = %q", unknown.(Model).commandMsg)
	}
	empty, cmd := (Model{runs: threeRuns(), commanding: true, command: "   "}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || empty.(Model).commanding {
		t.Errorf("empty command should be a no-op that closes the prompt")
	}
}

func TestLoadRuns_AppliesNameOverlay(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AKUAKU_STATE_DIR", dir)
	if err := state.Write(dir, state.Run{ID: "run-1", Name: "original", Status: state.StatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteName(dir, "run-1", "custom"); err != nil {
		t.Fatal(err)
	}
	msg := loadRuns(false).(runsMsg)
	if len(msg.runs) != 1 || msg.runs[0].Name != "custom" {
		t.Errorf("overlay name not applied: %+v", msg.runs)
	}
}

func TestDispatch_DiscoveryTogglesOnAndOff(t *testing.T) {
	on, _ := Model{runs: threeRuns()}.dispatch("discovery")
	if !on.(Model).discover {
		t.Fatal("discovery should be on after the first toggle")
	}
	if !strings.Contains(on.(Model).commandMsg, "discovery on") {
		t.Errorf("on message = %q", on.(Model).commandMsg)
	}

	off, _ := on.(Model).dispatch("discovery")
	if off.(Model).discover {
		t.Error("discovery should be off after the second toggle")
	}
	if !strings.Contains(off.(Model).commandMsg, "discovery off") {
		t.Errorf("off message = %q", off.(Model).commandMsg)
	}
}

func TestLoadRuns_MergesDiscoveredOnlyWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AKUAKU_STATE_DIR", dir)
	// An agent launched by Akuaku, recorded on disk with its PID.
	if err := state.Write(dir, state.Run{ID: "claude-1", Backend: "claude", Status: state.StatusRunning, PID: 100}); err != nil {
		t.Fatal(err)
	}
	restore := listProcesses
	defer func() { listProcesses = restore }()
	listProcesses = func() []state.Run {
		return []state.Run{
			{ID: "proc-100", Backend: "claude", Status: state.StatusRunning, PID: 100}, // same PID: deduped
			{ID: "proc-200", Backend: "codex", Status: state.StatusRunning, PID: 200},  // new: surfaced
		}
	}

	if msg := loadRuns(true).(runsMsg); len(msg.runs) != 2 {
		t.Fatalf("with discovery on want 2 runs (on-disk + new), got %d: %+v", len(msg.runs), msg.runs)
	}
	if msg := loadRuns(false).(runsMsg); len(msg.runs) != 1 {
		t.Fatalf("with discovery off want only the on-disk run, got %d", len(msg.runs))
	}
}

func TestReload_CapturesDiscoverFlagAndScans(t *testing.T) {
	t.Setenv("AKUAKU_STATE_DIR", t.TempDir())
	restore := listProcesses
	defer func() { listProcesses = restore }()
	listProcesses = func() []state.Run {
		return []state.Run{{ID: "proc-1", Backend: "claude", Status: state.StatusRunning, PID: 1}}
	}

	msg := Model{discover: true}.reload()().(runsMsg)
	if len(msg.runs) != 1 {
		t.Fatalf("reload with discovery on should include the discovered run, got %d", len(msg.runs))
	}
}

func TestListProcesses_DefaultIsInert(t *testing.T) {
	if got := listProcesses(); got != nil {
		t.Errorf("the default process source should return nil, got %+v", got)
	}
}

func TestSetProcessSource_WiresTheScanner(t *testing.T) {
	restore := listProcesses
	defer func() { listProcesses = restore }()

	SetProcessSource(func() []state.Run { return []state.Run{{PID: 7}} })
	if got := listProcesses(); len(got) != 1 || got[0].PID != 7 {
		t.Errorf("SetProcessSource did not wire the source: %+v", got)
	}
}

func TestView_FooterShowsCommandInputAndResult(t *testing.T) {
	editing := Model{runs: threeRuns(), width: 100, commanding: true, command: "rename foo"}.View()
	if !strings.Contains(editing, ":rename foo") {
		t.Errorf("command input not shown, got:\n%s", editing)
	}
	result := Model{runs: threeRuns(), width: 100, commandMsg: "renamed to foo"}.View()
	if !strings.Contains(result, "renamed to foo") {
		t.Errorf("command result not shown, got:\n%s", result)
	}
}

func TestView_DetailRendersAsConversation(t *testing.T) {
	runs := []state.Run{{Name: "review", Backend: "claude", Status: state.StatusDone,
		Task: "2 tips", Output: "1. small PRs"}}
	out := Model{runs: runs, detail: true, showAll: true}.View()
	for _, want := range []string{"You", "2 tips", "🗿 claude", "1. small PRs", "rename"} {
		if !strings.Contains(out, want) {
			t.Errorf("conversation detail missing %q, got:\n%s", want, out)
		}
	}
}

func TestView_DetailConversationFallbacksAndCommand(t *testing.T) {
	runs := []state.Run{{Name: "ext", Backend: "claude", Status: state.StatusRunning, Source: "hook"}}
	out := Model{runs: runs, detail: true, commanding: true, command: "rename x"}.View()
	if !strings.Contains(out, "no prompt recorded") || !strings.Contains(out, "no output captured") {
		t.Errorf("missing-content fallbacks not shown, got:\n%s", out)
	}
	if !strings.Contains(out, ":rename x") {
		t.Errorf("command input not shown in detail, got:\n%s", out)
	}
	done := Model{runs: runs, detail: true, commandMsg: "renamed to x"}.View()
	if !strings.Contains(done, "renamed to x") {
		t.Errorf("command result not shown in detail, got:\n%s", done)
	}
}

func TestDispatch_KillSignalsRunningAgent(t *testing.T) {
	t.Setenv("AKUAKU_STATE_DIR", t.TempDir())
	original := killProcess
	var killed int
	killProcess = func(pid int) error { killed = pid; return nil }
	defer func() { killProcess = original }()

	runs := []state.Run{{ID: "r", Name: "runaway", Status: state.StatusRunning, PID: 9999}}
	next, cmd := (Model{runs: runs, commanding: true, command: "kill"}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(next.(Model).commandMsg, "killing") {
		t.Errorf("commandMsg = %q", next.(Model).commandMsg)
	}
	if cmd == nil {
		t.Fatal("kill should return a command")
	}
	if _, ok := cmd().(runsMsg); !ok {
		t.Fatalf("kill cmd returned %T", cmd())
	}
	if killed != 9999 {
		t.Errorf("killed PID = %d, want 9999", killed)
	}
}

func TestDispatch_KillRefusesNonRunningNoPIDAndNoSelection(t *testing.T) {
	done := []state.Run{{ID: "d", Status: state.StatusDone, PID: 1, Name: "old"}}
	m1, cmd1 := (Model{runs: done, showAll: true, commanding: true, command: "kill"}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd1 != nil || !strings.Contains(m1.(Model).commandMsg, "only running") {
		t.Errorf("done run: msg = %q", m1.(Model).commandMsg)
	}

	hook := []state.Run{{ID: "h", Status: state.StatusRunning, Source: "hook", Name: "ext"}} // PID 0
	m2, cmd2 := (Model{runs: hook, commanding: true, command: "kill"}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd2 != nil || !strings.Contains(m2.(Model).commandMsg, "no process") {
		t.Errorf("hook run: msg = %q", m2.(Model).commandMsg)
	}

	m3, cmd3 := (Model{commanding: true, command: "kill"}).Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd3 != nil || !strings.Contains(m3.(Model).commandMsg, "no agent selected") {
		t.Errorf("no selection: msg = %q", m3.(Model).commandMsg)
	}
}

func TestUpdate_KArmsKillConfirmationWithoutMovingCursor(t *testing.T) {
	runs := []state.Run{{ID: "r", Name: "runaway", Status: state.StatusRunning, PID: 9999}}
	next, cmd := Model{runs: runs}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m := next.(Model)
	if !m.confirmKill || m.killPID != 9999 || m.killName != "runaway" {
		t.Fatalf("k should arm the kill: %+v", m)
	}
	if cmd != nil {
		t.Error("arming must not run a command yet")
	}
	if m.cursor != 0 {
		t.Error("k must not move the cursor (it is no longer an up alias)")
	}
}

func TestUpdate_KOnUnkillableRunReportsReason(t *testing.T) {
	hook := []state.Run{{ID: "h", Status: state.StatusRunning, Source: "hook", Name: "ext"}} // PID 0
	next, cmd := Model{runs: hook}.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if next.(Model).confirmKill {
		t.Error("must not arm a kill for a run with no process")
	}
	if cmd != nil || !strings.Contains(next.(Model).commandMsg, "no process") {
		t.Errorf("msg = %q", next.(Model).commandMsg)
	}
}

func TestConfirmKill_YSignalsTheArmedProcess(t *testing.T) {
	t.Setenv("AKUAKU_STATE_DIR", t.TempDir())
	original := killProcess
	var killed int
	killProcess = func(pid int) error { killed = pid; return nil }
	defer func() { killProcess = original }()

	next, cmd := Model{confirmKill: true, killPID: 9999, killName: "runaway"}.
		Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if next.(Model).confirmKill {
		t.Error("confirmation should clear after y")
	}
	if !strings.Contains(next.(Model).commandMsg, "killing") {
		t.Errorf("msg = %q", next.(Model).commandMsg)
	}
	if cmd == nil {
		t.Fatal("y should run the kill command")
	}
	if _, ok := cmd().(runsMsg); !ok {
		t.Fatalf("kill cmd returned %T", cmd())
	}
	if killed != 9999 {
		t.Errorf("killed PID = %d, want 9999", killed)
	}
}

func TestConfirmKill_OtherKeyCancels(t *testing.T) {
	next, cmd := Model{confirmKill: true, killPID: 9999, killName: "runaway"}.
		Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if next.(Model).confirmKill {
		t.Error("confirmation should clear after n")
	}
	if cmd != nil {
		t.Error("canceling must not run a command")
	}
	if !strings.Contains(next.(Model).commandMsg, "canceled") {
		t.Errorf("msg = %q", next.(Model).commandMsg)
	}
}

func TestView_FooterShowsKillConfirmation(t *testing.T) {
	out := Model{runs: threeRuns(), width: 100, confirmKill: true, killName: "two"}.View()
	if !strings.Contains(out, "kill") || !strings.Contains(out, "two") || !strings.Contains(out, "y confirm") {
		t.Errorf("footer should prompt to confirm the kill, got:\n%s", out)
	}
}

func TestKillProcess_TerminatesRealProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := killProcess(cmd.Process.Pid); err != nil {
		t.Fatalf("killProcess: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Error("expected the process to be terminated")
	}
}

var errBoom = boomError("boom")

type boomError string

func (e boomError) Error() string { return string(e) }
