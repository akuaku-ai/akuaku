package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNew_StartsWithZeroTicks(t *testing.T) {
	if got := New().ticks; got != 0 {
		t.Fatalf("New().ticks = %d, want 0", got)
	}
}

func TestInit_SchedulesFirstTick(t *testing.T) {
	if New().Init() == nil {
		t.Fatal("Init() = nil, want a tick command")
	}
}

func TestUpdate_TickIncrementsAndReschedules(t *testing.T) {
	updated, cmd := New().Update(newTickMsg(time.Unix(0, 0)))

	m, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	if m.ticks != 1 {
		t.Fatalf("ticks after one tick = %d, want 1", m.ticks)
	}
	if cmd == nil {
		t.Fatal("Update(tickMsg) returned nil cmd, want a reschedule")
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
	if updated.(Model).ticks != 0 {
		t.Fatal("model changed on a non-quit key")
	}
}

func TestUpdate_UnknownMessageIsIgnored(t *testing.T) {
	type otherMsg struct{}

	updated, cmd := New().Update(otherMsg{})

	if cmd != nil {
		t.Fatal("expected nil cmd for an unknown message")
	}
	if updated.(Model).ticks != 0 {
		t.Fatal("model changed on an unknown message")
	}
}

func TestView_RendersTitleAndTickCount(t *testing.T) {
	out := Model{ticks: 7}.View()

	if !strings.Contains(out, "Akuaku") {
		t.Fatalf("View() missing title, got %q", out)
	}
	if !strings.Contains(out, "7") {
		t.Fatalf("View() missing tick count, got %q", out)
	}
}

func TestNewTickMsg_WrapsTime(t *testing.T) {
	now := time.Unix(42, 0)

	if got := newTickMsg(now); got != tickMsg(now) {
		t.Fatalf("newTickMsg(%v) = %v, want %v", now, got, tickMsg(now))
	}
}

func TestTickCmd_ReturnsCommand(t *testing.T) {
	if tickCmd() == nil {
		t.Fatal("tickCmd() = nil, want a command")
	}
}
