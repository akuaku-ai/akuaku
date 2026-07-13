package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akuaku-ai/akuaku/internal/state"
)

func TestHandle_SessionStartCreatesRunningRun(t *testing.T) {
	dir := t.TempDir()
	in := `{"session_id":"s1","model":"claude-sonnet-5","session_title":"my session","cwd":"/home/u/proj"}`

	if err := Handle("SessionStart", strings.NewReader(in), dir, time.Unix(10, 0).UTC()); err != nil {
		t.Fatal(err)
	}

	run, found, _ := state.Read(dir, "s1")
	if !found {
		t.Fatal("run not created")
	}
	if run.Status != state.StatusRunning || run.Backend != "claude" || run.Source != "hook" {
		t.Errorf("wrong fields: %+v", run)
	}
	if run.Name != "my session" || run.Model != "claude-sonnet-5" {
		t.Errorf("name/model: %q/%q", run.Name, run.Model)
	}
	if run.Dir != "/home/u/proj" {
		t.Errorf("dir = %q, want the session's cwd so scope can place it", run.Dir)
	}
}

func TestHandle_SessionStartDefaultsName(t *testing.T) {
	dir := t.TempDir()
	if err := Handle("SessionStart", strings.NewReader(`{"session_id":"s1"}`), dir, time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
	run, _, _ := state.Read(dir, "s1")
	if run.Name != "claude session" {
		t.Errorf("name = %q, want default", run.Name)
	}
}

func TestHandle_UserPromptSetsTaskOnce(t *testing.T) {
	dir := t.TempDir()
	mustStart(t, dir, "s1")

	if err := Handle("UserPromptSubmit", strings.NewReader(`{"session_id":"s1","user_input":"refactor auth"}`), dir, time.Unix(11, 0)); err != nil {
		t.Fatal(err)
	}
	if run, _, _ := state.Read(dir, "s1"); run.Task != "refactor auth" {
		t.Fatalf("task = %q", run.Task)
	}

	// A later prompt must not overwrite the first.
	if err := Handle("UserPromptSubmit", strings.NewReader(`{"session_id":"s1","user_input":"something else"}`), dir, time.Unix(12, 0)); err != nil {
		t.Fatal(err)
	}
	if run, _, _ := state.Read(dir, "s1"); run.Task != "refactor auth" {
		t.Errorf("task overwritten: %q", run.Task)
	}
}

func TestHandle_SessionEndCompletesRun(t *testing.T) {
	dir := t.TempDir()
	mustStart(t, dir, "s1")

	if err := Handle("SessionEnd", strings.NewReader(`{"session_id":"s1"}`), dir, time.Unix(30, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	run, _, _ := state.Read(dir, "s1")
	if run.Status != state.StatusDone || run.EndedAt == nil {
		t.Fatalf("not completed: %+v", run)
	}
	if run.StartedAt.IsZero() {
		t.Error("start time not preserved across end")
	}
}

func TestHandle_MalformedInputIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := Handle("SessionStart", strings.NewReader("not json"), dir, time.Unix(1, 0)); err != nil {
		t.Fatalf("malformed input should be a no-op, got %v", err)
	}
	if runs, _ := state.ReadDir(dir); len(runs) != 0 {
		t.Error("no run should be written")
	}
}

func TestHandle_MissingSessionIDIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := Handle("SessionStart", strings.NewReader(`{}`), dir, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if runs, _ := state.ReadDir(dir); len(runs) != 0 {
		t.Error("no run without a session_id")
	}
}

func TestHandle_UnknownEventIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := Handle("Stop", strings.NewReader(`{"session_id":"s1"}`), dir, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if runs, _ := state.ReadDir(dir); len(runs) != 0 {
		t.Error("an unknown event should not write")
	}
}

func TestHandle_EndOnMissingRunIsNoOp(t *testing.T) {
	dir := t.TempDir()
	if err := Handle("SessionEnd", strings.NewReader(`{"session_id":"ghost"}`), dir, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if runs, _ := state.ReadDir(dir); len(runs) != 0 {
		t.Error("ending a missing run should not create one")
	}
}

func TestHandle_CorruptExistingRunErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "s1.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Handle("SessionEnd", strings.NewReader(`{"session_id":"s1"}`), dir, time.Unix(1, 0)); err == nil {
		t.Fatal("expected an error from a corrupt existing run")
	}
}

func mustStart(t *testing.T, dir, id string) {
	t.Helper()
	if err := Handle("SessionStart", strings.NewReader(`{"session_id":"`+id+`"}`), dir, time.Unix(10, 0)); err != nil {
		t.Fatal(err)
	}
}
