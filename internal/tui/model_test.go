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

var errBoom = boomError("boom")

type boomError string

func (e boomError) Error() string { return string(e) }
