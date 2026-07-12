package launcher

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/akuaku-ai/akuaku/internal/state"
)

// fakeRunner returns fixed output for a subprocess and reports a fixed PID.
func fakeRunner(stdout, stderr string, exitCode int, err error) commandRunner {
	return func(_ string, _ []string, onStart func(int)) ([]byte, []byte, int, error) {
		onStart(4242)
		return []byte(stdout), []byte(stderr), exitCode, err
	}
}

// capturingLauncher builds a Launcher with injected deps and returns it plus a
// pointer to the slice of runs handed to its writer.
func capturingLauncher(t *testing.T, run commandRunner) (*Launcher, *[]state.Run) {
	t.Helper()
	var written []state.Run
	l := &Launcher{
		run:    run,
		write:  func(_ string, r state.Run) error { written = append(written, r); return nil },
		now:    func() time.Time { return time.Unix(1000, 0).UTC() },
		suffix: func() (string, error) { return "abcd", nil },
		out:    &bytes.Buffer{},
	}
	return l, &written
}

// printed returns what the launcher wrote to its output writer.
func printed(l *Launcher) string { return l.out.(*bytes.Buffer).String() }

func TestRun_SuccessRecordsDoneWithUsage(t *testing.T) {
	l, written := capturingLauncher(t, fakeRunner(`{"total_cost_usd":0.5,"usage":{"input_tokens":3,"output_tokens":7}}`, "", 0, nil))

	if err := l.Run(Options{Backend: "claude", Task: "do it", Dir: "d"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(*written) != 2 {
		t.Fatalf("expected running then terminal write, got %d", len(*written))
	}
	if (*written)[0].Status != state.StatusRunning || (*written)[0].PID != 4242 {
		t.Errorf("first write should be running with the PID, got %+v", (*written)[0])
	}
	done := (*written)[1]
	if done.Status != state.StatusDone {
		t.Errorf("status = %q, want done", done.Status)
	}
	if done.Tokens != 10 || done.Cost != 0.5 {
		t.Errorf("usage not recorded: %d tokens, %v cost", done.Tokens, done.Cost)
	}
	if done.ExitCode == nil || *done.ExitCode != 0 || done.EndedAt == nil {
		t.Errorf("terminal fields not set: %+v", done)
	}
}

func TestRun_PrintsAndRecordsAnswer(t *testing.T) {
	l, written := capturingLauncher(t, fakeRunner(`{"result":"hi there","usage":{"input_tokens":1,"output_tokens":1}}`, "", 0, nil))

	if err := l.Run(Options{Backend: "claude", Task: "t", Dir: "d"}); err != nil {
		t.Fatal(err)
	}
	if got := (*written)[1].Output; got != "hi there" {
		t.Errorf("answer not recorded in state: %q", got)
	}
	out := printed(l)
	if !strings.Contains(out, "running claude") {
		t.Errorf("run should be announced, got:\n%s", out)
	}
	if !strings.Contains(out, "hi there") {
		t.Errorf("answer should be printed, got:\n%s", out)
	}
}

func TestRun_PrintsErrorOnFailure(t *testing.T) {
	l, _ := capturingLauncher(t, fakeRunner("", "boom happened", 2, nil))

	_ = l.Run(Options{Backend: "claude", Task: "t", Dir: "d"})
	out := printed(l)
	if !strings.Contains(out, "error") || !strings.Contains(out, "boom happened") {
		t.Errorf("failure should be reported, got:\n%s", out)
	}
}

func TestRun_NonZeroExitRecordsErrorWithStderr(t *testing.T) {
	l, written := capturingLauncher(t, fakeRunner("", "boom happened", 2, nil))

	if err := l.Run(Options{Backend: "claude", Task: "t", Dir: "d"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := (*written)[1]
	if got.Status != state.StatusError || *got.ExitCode != 2 {
		t.Errorf("want error/exit 2, got %q/%v", got.Status, got.ExitCode)
	}
	if got.Error != "boom happened" {
		t.Errorf("error message = %q", got.Error)
	}
}

func TestRun_NonZeroExitWithoutStderrHasDefaultMessage(t *testing.T) {
	l, written := capturingLauncher(t, fakeRunner("", "", 3, nil))

	_ = l.Run(Options{Backend: "claude", Task: "t", Dir: "d"})
	if got := (*written)[1]; got.Error == "" {
		t.Error("expected a default error message")
	}
}

func TestRun_StartFailureRecordsError(t *testing.T) {
	l, written := capturingLauncher(t, fakeRunner("", "", -1, errors.New("not found")))

	if err := l.Run(Options{Backend: "claude", Task: "t", Dir: "d"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := (*written)[1]
	if got.Status != state.StatusError || got.Error != "not found" {
		t.Errorf("want error/'not found', got %q/%q", got.Status, got.Error)
	}
}

func TestRun_UnknownBackendReturnsError(t *testing.T) {
	l, _ := capturingLauncher(t, fakeRunner("", "", 0, nil))
	if err := l.Run(Options{Backend: "nope", Task: "t", Dir: "d"}); err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}

func TestRun_SuffixErrorReturnsError(t *testing.T) {
	l, _ := capturingLauncher(t, fakeRunner("", "", 0, nil))
	l.suffix = func() (string, error) { return "", errors.New("no randomness") }
	if err := l.Run(Options{Backend: "claude", Task: "t", Dir: "d"}); err == nil {
		t.Fatal("expected the suffix error to propagate")
	}
}

func TestRun_FirstWriteErrorReturnsError(t *testing.T) {
	l, _ := capturingLauncher(t, fakeRunner("", "", 0, nil))
	l.write = func(string, state.Run) error { return errors.New("disk full") }
	if err := l.Run(Options{Backend: "claude", Task: "t", Dir: "d"}); err == nil {
		t.Fatal("expected the first write error to propagate")
	}
}

func TestRun_SecondWriteErrorReturnsError(t *testing.T) {
	l, _ := capturingLauncher(t, fakeRunner("", "", 0, nil))
	calls := 0
	l.write = func(string, state.Run) error {
		calls++
		if calls == 2 {
			return errors.New("disk full")
		}
		return nil
	}
	if err := l.Run(Options{Backend: "claude", Task: "t", Dir: "d"}); err == nil {
		t.Fatal("expected the second write error to propagate")
	}
}

func TestRun_ExplicitNameOverridesTask(t *testing.T) {
	l, written := capturingLauncher(t, fakeRunner("{}", "", 0, nil))
	_ = l.Run(Options{Backend: "claude", Task: "the task", Name: "my agent", Dir: "d"})
	if (*written)[0].Name != "my agent" {
		t.Errorf("name = %q, want 'my agent'", (*written)[0].Name)
	}
}

func TestRun_DefaultsNameToTask(t *testing.T) {
	l, written := capturingLauncher(t, fakeRunner("{}", "", 0, nil))
	_ = l.Run(Options{Backend: "claude", Task: "the task", Dir: "d"})
	if (*written)[0].Name != "the task" {
		t.Errorf("name = %q, want 'the task'", (*written)[0].Name)
	}
}

func TestExecRun_Success(t *testing.T) {
	gotPID := 0
	stdout, _, code, err := execRun("echo", []string{"hello"}, func(pid int) { gotPID = pid })
	if err != nil || code != 0 {
		t.Fatalf("echo failed: code %d, err %v", code, err)
	}
	if !strings.Contains(string(stdout), "hello") {
		t.Errorf("stdout = %q", stdout)
	}
	if gotPID <= 0 {
		t.Errorf("onStart should report a real PID, got %d", gotPID)
	}
}

func TestExecRun_NonZeroExit(t *testing.T) {
	_, _, code, err := execRun("false", nil, func(int) {})
	if err != nil {
		t.Fatalf("false should not error, got %v", err)
	}
	if code == 0 {
		t.Errorf("expected a non-zero exit code")
	}
}

func TestExecRun_CapturesStderr(t *testing.T) {
	_, stderr, _, err := execRun("sh", []string{"-c", "echo oops 1>&2"}, func(int) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(stderr), "oops") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestExecRun_StartFailure(t *testing.T) {
	_, _, code, err := execRun("akuaku-does-not-exist-xyz", nil, func(int) {
		t.Error("onStart must not be called when the process fails to start")
	})
	if err == nil {
		t.Fatal("expected an error for a missing command")
	}
	if code != -1 {
		t.Errorf("exit code = %d, want -1", code)
	}
}

func TestNew_WiresRealDependencies(t *testing.T) {
	l := New(io.Discard)
	if l.run == nil || l.write == nil {
		t.Fatal("runner/writer not wired")
	}
	if l.out == nil {
		t.Error("output writer not wired")
	}
	if s, err := l.suffix(); err != nil || s == "" {
		t.Errorf("suffix closure failed: %q, %v", s, err)
	}
	if l.now().IsZero() {
		t.Error("clock not wired")
	}
}
