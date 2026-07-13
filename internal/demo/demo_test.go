package demo_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/akuaku-ai/akuaku/internal/demo"
	"github.com/akuaku-ai/akuaku/internal/state"
)

var base = time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)

// byID indexes a frame's runs so a test can assert on one agent.
func byID(runs []state.Run) map[string]state.Run {
	index := make(map[string]state.Run, len(runs))
	for _, run := range runs {
		index[run.ID] = run
	}
	return index
}

func TestFrame_OpensWithThreeRunningAndOneWaiting(t *testing.T) {
	runs := byID(demo.Frame(0, base))
	if len(runs) != 4 {
		t.Fatalf("phase 0 should show 4 agents, got %d", len(runs))
	}
	if runs["demo-a"].Status != state.StatusRunning || runs["demo-b"].Status != state.StatusRunning || runs["demo-c"].Status != state.StatusRunning {
		t.Error("a, b, c should be running at the start")
	}
	if runs["demo-d"].Status != state.StatusWaiting {
		t.Errorf("d should be waiting, got %q", runs["demo-d"].Status)
	}
	if _, ok := runs["demo-e"]; ok {
		t.Error("e should not have launched yet")
	}
}

func TestFrame_ClaudeRefactorFinishesAtPhase6(t *testing.T) {
	if byID(demo.Frame(5, base))["demo-a"].Status != state.StatusRunning {
		t.Error("a should still be running at phase 5")
	}
	done := byID(demo.Frame(6, base))["demo-a"]
	if done.Status != state.StatusDone {
		t.Errorf("a should be done at phase 6, got %q", done.Status)
	}
	if done.EndedAt == nil {
		t.Error("a done should carry an end time so its duration freezes")
	}
}

func TestFrame_NewAgentLaunchesAtPhase9(t *testing.T) {
	if _, ok := byID(demo.Frame(8, base))["demo-e"]; ok {
		t.Error("e should be absent at phase 8")
	}
	e, ok := byID(demo.Frame(9, base))["demo-e"]
	if !ok || e.Status != state.StatusRunning {
		t.Errorf("e should appear running at phase 9, got %+v", e)
	}
}

func TestFrame_OllamaWaitsAtPhase12(t *testing.T) {
	if byID(demo.Frame(11, base))["demo-c"].Status != state.StatusRunning {
		t.Error("c should be running at phase 11")
	}
	if byID(demo.Frame(12, base))["demo-c"].Status != state.StatusWaiting {
		t.Error("c should be waiting at phase 12")
	}
}

func TestFrame_ReviewResumesAtPhase9(t *testing.T) {
	if byID(demo.Frame(8, base))["demo-d"].Status != state.StatusWaiting {
		t.Error("d should be waiting at phase 8")
	}
	if byID(demo.Frame(9, base))["demo-d"].Status != state.StatusRunning {
		t.Error("d should resume running at phase 9")
	}
}

func TestFrame_LoopsWithPeriod(t *testing.T) {
	if !reflect.DeepEqual(demo.Frame(0, base), demo.Frame(demo.Period, base)) {
		t.Error("the frame at Period should equal the frame at 0, so the GIF loops seamlessly")
	}
	if !reflect.DeepEqual(demo.Frame(3, base), demo.Frame(demo.Period+3, base)) {
		t.Error("frames should be periodic")
	}
}

func TestFrame_NegativeTickIsHandled(t *testing.T) {
	if !reflect.DeepEqual(demo.Frame(-1, base), demo.Frame(demo.Period-1, base)) {
		t.Error("a negative tick should wrap like any other")
	}
}

func TestFrame_TokensClimbWhileRunning(t *testing.T) {
	early := byID(demo.Frame(0, base))["demo-b"].Tokens
	later := byID(demo.Frame(4, base))["demo-b"].Tokens
	if later <= early {
		t.Errorf("running tokens should climb: %d then %d", early, later)
	}
}

func TestFrame_OnlyClaudeReportsCost(t *testing.T) {
	runs := byID(demo.Frame(0, base))
	if runs["demo-a"].Cost <= 0 {
		t.Error("claude should report a cost")
	}
	if runs["demo-b"].Cost != 0 || runs["demo-c"].Cost != 0 {
		t.Error("codex and ollama report no cost")
	}
}

func TestFrame_StartTimesPrecedeBaseForRealisticDurations(t *testing.T) {
	for _, run := range demo.Frame(0, base) {
		if !run.StartedAt.Before(base) {
			t.Errorf("%s should have started before now so its duration is non-zero", run.ID)
		}
	}
}
