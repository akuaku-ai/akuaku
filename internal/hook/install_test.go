package hook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// readInstalled parses the hooks section written to path.
func readInstalled(t *testing.T, path string) map[string][]hookGroup {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var typed struct {
		Hooks map[string][]hookGroup `json:"hooks"`
	}
	if err := json.Unmarshal(data, &typed); err != nil {
		t.Fatalf("settings not valid JSON: %v", err)
	}
	return typed.Hooks
}

// countCommand counts how many times command appears across an event's groups.
func countCommand(groups []hookGroup, command string) int {
	n := 0
	for _, g := range groups {
		for _, entry := range g.Hooks {
			if entry.Command == command {
				n++
			}
		}
	}
	return n
}

func TestInstall_FreshFileCreatesAllHooks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := Install(path, "akuaku"); err != nil {
		t.Fatal(err)
	}
	hooks := readInstalled(t, path)
	for _, event := range installEvents {
		if !hasCommand(hooks[event], "akuaku hook "+event) {
			t.Errorf("missing hook for %s", event)
		}
	}
}

func TestInstall_EmptyFileTreatedAsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(path, "akuaku"); err != nil {
		t.Fatal(err)
	}
	hooks := readInstalled(t, path)
	if !hasCommand(hooks["SessionStart"], "akuaku hook SessionStart") {
		t.Error("empty file should be treated as fresh")
	}
}

func TestInstall_PreservesUnrelatedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo hi","timeout":5}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(path, "akuaku"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	if root["model"] != "opus" {
		t.Errorf("unrelated setting lost: model = %v", root["model"])
	}

	hooks := readInstalled(t, path)
	pre := hooks["PreToolUse"]
	if len(pre) != 1 || pre[0].Matcher != "Bash" {
		t.Fatalf("PreToolUse group not preserved: %+v", pre)
	}
	if len(pre[0].Hooks) != 1 || pre[0].Hooks[0].Command != "echo hi" || pre[0].Hooks[0].Timeout != 5 {
		t.Errorf("existing hook entry not preserved: %+v", pre[0].Hooks)
	}
	for _, event := range installEvents {
		if !hasCommand(hooks[event], "akuaku hook "+event) {
			t.Errorf("missing hook for %s", event)
		}
	}
}

func TestInstall_PreservesExistingHooksOnSameEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	existing := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo start"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Install(path, "akuaku"); err != nil {
		t.Fatal(err)
	}

	hooks := readInstalled(t, path)
	if !hasCommand(hooks["SessionStart"], "echo start") {
		t.Error("dropped the user's own SessionStart hook")
	}
	if !hasCommand(hooks["SessionStart"], "akuaku hook SessionStart") {
		t.Error("did not add the Akuaku hook alongside it")
	}
}

func TestInstall_IsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := Install(path, "akuaku"); err != nil {
		t.Fatal(err)
	}
	if err := Install(path, "akuaku"); err != nil {
		t.Fatal(err)
	}
	hooks := readInstalled(t, path)
	for _, event := range installEvents {
		if n := countCommand(hooks[event], "akuaku hook "+event); n != 1 {
			t.Errorf("%s appears %d times, want exactly 1", event, n)
		}
	}
}

func TestInstall_ReadErrorPropagates(t *testing.T) {
	// A directory in place of the settings file makes ReadFile fail (not ENOENT).
	dir := t.TempDir()
	if err := Install(dir, "akuaku"); err == nil {
		t.Fatal("expected a read error")
	}
}

func TestInstall_MalformedSettingsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(path, "akuaku"); err == nil {
		t.Fatal("expected a parse error on malformed settings")
	}
}

func TestInstall_MalformedHooksErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"hooks":5}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(path, "akuaku"); err == nil {
		t.Fatal("expected an error when hooks has the wrong shape")
	}
}

func TestInstall_MarshalError(t *testing.T) {
	original := marshal
	marshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { marshal = original }()

	if err := Install(filepath.Join(t.TempDir(), "settings.json"), "akuaku"); err == nil {
		t.Fatal("expected a marshal error")
	}
}

func TestInstall_WriteFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	// Occupy the temporary path with a directory so WriteFile cannot create it.
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(path, "akuaku"); err == nil {
		t.Fatal("expected a WriteFile error")
	}
}

func TestInstall_RenameError(t *testing.T) {
	original := rename
	rename = func(string, string) error { return errors.New("boom") }
	defer func() { rename = original }()

	if err := Install(filepath.Join(t.TempDir(), "settings.json"), "akuaku"); err == nil {
		t.Fatal("expected a rename error")
	}
}
