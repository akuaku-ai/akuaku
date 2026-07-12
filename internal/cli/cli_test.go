package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/akuaku-ai/akuaku/internal/launcher"
)

// harness builds Deps that record what was invoked.
type harness struct {
	deps          Deps
	out, err      bytes.Buffer
	launched      []launcher.Options
	monitorCalled bool
}

func newHarness(monitorErr, launchErr error) *harness {
	h := &harness{}
	h.deps = Deps{
		Monitor: func() error { h.monitorCalled = true; return monitorErr },
		Launch: func(o launcher.Options) error {
			h.launched = append(h.launched, o)
			return launchErr
		},
		Out: &h.out,
		Err: &h.err,
	}
	return h
}

func TestRun_NoArgsRunsMonitor(t *testing.T) {
	h := newHarness(nil, nil)
	if code := Run(nil, h.deps); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !h.monitorCalled {
		t.Error("monitor was not run")
	}
}

func TestRun_MonitorErrorExitsNonZero(t *testing.T) {
	h := newHarness(errors.New("tty gone"), nil)
	if code := Run(nil, h.deps); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(h.err.String(), "tty gone") {
		t.Errorf("stderr = %q", h.err.String())
	}
}

func TestRun_LaunchesWithBackendAndTask(t *testing.T) {
	h := newHarness(nil, nil)
	code := Run([]string{"run", "claude", "do it"}, h.deps)
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if len(h.launched) != 1 || h.launched[0].Backend != "claude" || h.launched[0].Task != "do it" {
		t.Fatalf("bad launch: %+v", h.launched)
	}
	if h.launched[0].Dir == "" {
		t.Error("state dir not set")
	}
}

func TestRun_ParsesModelAndName(t *testing.T) {
	h := newHarness(nil, nil)
	Run([]string{"run", "claude", "task", "--model", "opus", "--name", "bot"}, h.deps)
	got := h.launched[0]
	if got.Model != "opus" || got.Name != "bot" {
		t.Errorf("model/name = %q/%q", got.Model, got.Name)
	}
}

func TestRun_ParsesShortFlagsBeforePositional(t *testing.T) {
	h := newHarness(nil, nil)
	Run([]string{"run", "-m", "llama3.1", "-n", "x", "ollama", "task"}, h.deps)
	got := h.launched[0]
	if got.Backend != "ollama" || got.Model != "llama3.1" || got.Name != "x" {
		t.Errorf("parsed wrong: %+v", got)
	}
}

func TestRun_MissingTaskShowsUsage(t *testing.T) {
	h := newHarness(nil, nil)
	if code := Run([]string{"run", "claude"}, h.deps); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(h.err.String(), "usage") {
		t.Errorf("stderr = %q", h.err.String())
	}
}

func TestRun_FlagWithoutValueErrors(t *testing.T) {
	h := newHarness(nil, nil)
	if code := Run([]string{"run", "claude", "task", "--model"}, h.deps); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if len(h.launched) != 0 {
		t.Error("should not launch on a parse error")
	}
}

func TestRun_LaunchErrorExitsNonZero(t *testing.T) {
	h := newHarness(nil, errors.New("unknown backend"))
	if code := Run([]string{"run", "bogus", "task"}, h.deps); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(h.err.String(), "unknown backend") {
		t.Errorf("stderr = %q", h.err.String())
	}
}

func TestRun_HelpShowsUsage(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		h := newHarness(nil, nil)
		if code := Run([]string{arg}, h.deps); code != 0 {
			t.Errorf("%s: code = %d, want 0", arg, code)
		}
		if !strings.Contains(h.out.String(), "akuaku") {
			t.Errorf("%s: usage missing, got %q", arg, h.out.String())
		}
	}
}

func TestRun_UnknownCommandErrors(t *testing.T) {
	h := newHarness(nil, nil)
	if code := Run([]string{"frobnicate"}, h.deps); code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(h.err.String(), "unknown command") {
		t.Errorf("stderr = %q", h.err.String())
	}
}
