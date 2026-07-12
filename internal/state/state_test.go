package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestRun_RunningStatusSerializesNullTerminalFields(t *testing.T) {
	run := Run{
		ID:        "claude-1-ab",
		Backend:   "claude",
		Status:    StatusRunning,
		Task:      "refactor auth",
		StartedAt: time.Unix(1, 0).UTC(),
	}

	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(data)

	if !strings.Contains(out, `"ended_at": null`) {
		t.Errorf("running run should have null ended_at, got:\n%s", out)
	}
	if !strings.Contains(out, `"exit_code": null`) {
		t.Errorf("running run should have null exit_code, got:\n%s", out)
	}
	if !strings.Contains(out, `"started_at": "1970-01-01T00:00:01Z"`) {
		t.Errorf("started_at should be RFC 3339, got:\n%s", out)
	}
}

func TestRun_JSONRoundTripPreservesTerminalFields(t *testing.T) {
	ended := time.Unix(90, 0).UTC()
	code := 0
	run := Run{
		ID:        "codex-2-cd",
		Backend:   "codex",
		Status:    StatusDone,
		StartedAt: time.Unix(1, 0).UTC(),
		EndedAt:   &ended,
		Tokens:    1200,
		Cost:      0.04,
		ExitCode:  &code,
	}

	data, err := json.Marshal(run)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Run
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.EndedAt == nil || !got.EndedAt.Equal(ended) {
		t.Errorf("ended_at not preserved: %v", got.EndedAt)
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("exit_code not preserved: %v", got.ExitCode)
	}
	if got.Tokens != 1200 || got.Cost != 0.04 {
		t.Errorf("tokens/cost not preserved: %d %v", got.Tokens, got.Cost)
	}
}

func TestDir_DefaultsToHomeAkuakuState(t *testing.T) {
	t.Setenv(envStateDir, "")
	t.Setenv("HOME", "/tmp/fake-home")
	want := filepath.Join("/tmp/fake-home", ".akuaku", "state")
	if got := Dir(); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestDir_FallsBackWhenHomeUnavailable(t *testing.T) {
	t.Setenv(envStateDir, "")
	t.Setenv("HOME", "")
	if got := Dir(); got != defaultStateDir {
		t.Errorf("Dir() = %q, want fallback %q", got, defaultStateDir)
	}
}

func TestDir_HonorsEnvOverride(t *testing.T) {
	t.Setenv(envStateDir, "/tmp/custom-akuaku")
	if got := Dir(); got != "/tmp/custom-akuaku" {
		t.Errorf("Dir() = %q, want the override", got)
	}
}

func TestNewID_Format(t *testing.T) {
	got := NewID("claude", time.Unix(0, 5), "abcd")
	if got != "claude-5-abcd" {
		t.Errorf("NewID = %q, want claude-5-abcd", got)
	}
}

func TestRandomSuffix_HexEncodesBytes(t *testing.T) {
	got, err := RandomSuffix(bytes.NewReader([]byte{0x00, 0x01, 0x0a, 0xff}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "00010aff" {
		t.Errorf("RandomSuffix = %q, want 00010aff", got)
	}
}

func TestRandomSuffix_PropagatesReaderError(t *testing.T) {
	if _, err := RandomSuffix(iotest.ErrReader(errors.New("boom"))); err == nil {
		t.Fatal("expected an error from a failing reader")
	}
}

func TestWrite_WritesFileAtomically(t *testing.T) {
	dir := t.TempDir()
	run := Run{ID: "claude-1-ab", Backend: "claude", Status: StatusRunning, StartedAt: time.Unix(1, 0).UTC()}

	if err := Write(dir, run); err != nil {
		t.Fatalf("Write: %v", err)
	}

	final := filepath.Join(dir, "claude-1-ab.json")
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var got Run
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != run.ID || got.Status != run.Status {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if _, err := os.Stat(final + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temporary file was left behind")
	}
}

func TestWrite_CreatesMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "state")
	if err := Write(dir, Run{ID: "x"}); err != nil {
		t.Fatalf("Write should create the directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.json")); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestWrite_MkdirAllError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file cannot have a child directory, so MkdirAll fails.
	if err := Write(filepath.Join(file, "sub"), Run{ID: "x"}); err == nil {
		t.Fatal("expected an error when the parent path is a file")
	}
}

func TestWrite_MarshalError(t *testing.T) {
	original := marshal
	marshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { marshal = original }()

	if err := Write(t.TempDir(), Run{ID: "x"}); err == nil {
		t.Fatal("expected a marshal error")
	}
}

func TestWrite_WriteFileError(t *testing.T) {
	dir := t.TempDir()
	// Occupy the temporary path with a directory so WriteFile cannot create it.
	if err := os.Mkdir(filepath.Join(dir, "x.json.tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, Run{ID: "x"}); err == nil {
		t.Fatal("expected a WriteFile error")
	}
}

func TestWrite_RenameError(t *testing.T) {
	dir := t.TempDir()
	// Occupy the final path with a non-empty directory so Rename fails.
	target := filepath.Join(dir, "x.json")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "inner"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, Run{ID: "x"}); err == nil {
		t.Fatal("expected a Rename error")
	}
}

func TestReadDir_MissingDirectoryIsEmpty(t *testing.T) {
	runs, err := ReadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected no runs, got %d", len(runs))
	}
}

func TestReadDir_NonDirectoryErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDir(file); err == nil {
		t.Fatal("expected an error reading a non-directory")
	}
}

func TestReadDir_ParsesValidAndSkipsTheRest(t *testing.T) {
	dir := t.TempDir()
	good := Run{ID: "claude-1-a", Backend: "claude", Status: StatusDone, StartedAt: time.Unix(1, 0).UTC()}
	if err := Write(dir, good); err != nil {
		t.Fatal(err)
	}
	// Unparseable JSON, a non-JSON file, a subdirectory, and a broken symlink
	// must all be skipped without failing the scan.
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "broken.json")); err != nil {
		t.Fatal(err)
	}

	runs, err := ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "claude-1-a" {
		t.Fatalf("expected only the valid run, got %+v", runs)
	}
}

func TestRun_SourceSerializesWhenSetAndOmittedWhenEmpty(t *testing.T) {
	withSource, err := json.Marshal(Run{Source: "hook"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withSource), `"source":"hook"`) {
		t.Errorf("source not serialized: %s", withSource)
	}

	noSource, err := json.Marshal(Run{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(noSource), "source") {
		t.Errorf("empty source should be omitted: %s", noSource)
	}
}

func TestRun_OutputSerializesWhenSetAndOmittedWhenEmpty(t *testing.T) {
	withOutput, err := json.Marshal(Run{Output: "the answer"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(withOutput), `"output":"the answer"`) {
		t.Errorf("output not serialized: %s", withOutput)
	}

	noOutput, err := json.Marshal(Run{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(noOutput), "output") {
		t.Errorf("empty output should be omitted: %s", noOutput)
	}
}

func TestRead_ReturnsStoredRun(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Run{ID: "claude-1-a", Status: StatusRunning, Source: "hook"}); err != nil {
		t.Fatal(err)
	}

	got, found, err := Read(dir, "claude-1-a")
	if err != nil || !found {
		t.Fatalf("Read: found=%v err=%v", found, err)
	}
	if got.ID != "claude-1-a" || got.Source != "hook" {
		t.Errorf("wrong run: %+v", got)
	}
}

func TestRead_MissingIsNotFound(t *testing.T) {
	got, found, err := Read(t.TempDir(), "nope")
	if err != nil {
		t.Fatalf("missing run should not error: %v", err)
	}
	if found {
		t.Errorf("expected not found, got %+v", got)
	}
}

func TestRead_ReadErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	// Occupy the path with a directory so ReadFile fails with a non-ENOENT error.
	if err := os.Mkdir(filepath.Join(dir, "x.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(dir, "x"); err == nil {
		t.Fatal("expected a read error")
	}
}

func TestRead_UnparseableErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(dir, "x"); err == nil {
		t.Fatal("expected an unmarshal error")
	}
}
