package setup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lookPathAll is a LookPath seam that reports every backend as installed.
func lookPathAll(string) (string, error) { return "/usr/bin/x", nil }

func TestRun_AlreadyOnPath_LeavesProfileUntouched(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(profile, []byte("# my config\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cfg := Config{BinDir: "/opt/bin", Path: "/opt/bin:/usr/bin", Profile: profile, Backends: []string{"claude"}, LookPath: lookPathAll}
	if err := Run(cfg, &out); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(profile)
	if string(data) != "# my config\n" {
		t.Errorf("profile should be untouched, got %q", data)
	}
	if !strings.Contains(out.String(), "on your PATH") {
		t.Errorf("out should confirm PATH, got:\n%s", out.String())
	}
}

func TestRun_NotOnPath_AppendsExport(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, ".zshrc")

	var out bytes.Buffer
	cfg := Config{BinDir: "/home/u/go/bin", Path: "/usr/bin", Profile: profile, Backends: []string{"claude"}, LookPath: lookPathAll}
	if err := Run(cfg, &out); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(profile)
	if !strings.Contains(string(data), `export PATH="$PATH:/home/u/go/bin"`) {
		t.Errorf("export line not appended, got %q", data)
	}
	if !strings.Contains(out.String(), profile) {
		t.Errorf("out should name the profile, got:\n%s", out.String())
	}
}

func TestRun_NotOnPath_DoesNotDuplicate(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, ".zshrc")
	cfg := Config{BinDir: "/home/u/go/bin", Path: "/usr/bin", Profile: profile, LookPath: lookPathAll}

	var out bytes.Buffer
	if err := Run(cfg, &out); err != nil {
		t.Fatal(err)
	}
	// A second run must not add the export again.
	if err := Run(cfg, &out); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(profile)
	if n := strings.Count(string(data), `export PATH="$PATH:/home/u/go/bin"`); n != 1 {
		t.Errorf("export appears %d times, want exactly 1", n)
	}
	if !strings.Contains(out.String(), "already in") {
		t.Errorf("second run should note the line is already present, got:\n%s", out.String())
	}
}

func TestRun_ProfileReadErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	// A directory in place of the profile makes ReadFile fail (not ENOENT).
	profile := filepath.Join(dir, "prof")
	if err := os.Mkdir(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := Config{BinDir: "/x/bin", Path: "/usr/bin", Profile: profile, LookPath: lookPathAll}
	if err := Run(cfg, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a read error")
	}
}

func TestRun_ProfileWriteErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	// The parent directory does not exist, so writing the temp file fails.
	profile := filepath.Join(dir, "nodir", ".zshrc")
	cfg := Config{BinDir: "/x/bin", Path: "/usr/bin", Profile: profile, LookPath: lookPathAll}
	if err := Run(cfg, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a write error")
	}
}

func TestRun_ProfileRenameErrorPropagates(t *testing.T) {
	original := rename
	rename = func(string, string) error { return errors.New("boom") }
	defer func() { rename = original }()

	dir := t.TempDir()
	profile := filepath.Join(dir, ".zshrc")
	cfg := Config{BinDir: "/x/bin", Path: "/usr/bin", Profile: profile, LookPath: lookPathAll}
	if err := Run(cfg, &bytes.Buffer{}); err == nil {
		t.Fatal("expected a rename error")
	}
}

func TestRun_ReportsBackends(t *testing.T) {
	look := func(name string) (string, error) {
		if name == "claude" {
			return "/usr/bin/claude", nil
		}
		return "", errors.New("nope")
	}
	var out bytes.Buffer
	cfg := Config{BinDir: "/opt/bin", Path: "/opt/bin", Backends: []string{"claude", "codex"}, LookPath: look}
	if err := Run(cfg, &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "✓ claude") {
		t.Errorf("installed backend not marked, got:\n%s", s)
	}
	if !strings.Contains(s, "✗ codex") {
		t.Errorf("missing backend not marked, got:\n%s", s)
	}
}

func TestProfileFor(t *testing.T) {
	home := "/home/u"
	cases := map[string]string{
		"/bin/zsh":      filepath.Join(home, ".zshrc"),
		"/bin/bash":     filepath.Join(home, ".bashrc"),
		"/usr/bin/fish": filepath.Join(home, ".profile"),
		"":              filepath.Join(home, ".profile"),
	}
	for shell, want := range cases {
		if got := ProfileFor(shell, home); got != want {
			t.Errorf("ProfileFor(%q) = %q, want %q", shell, got, want)
		}
	}
}
