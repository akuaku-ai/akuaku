// Package setup implements `akuaku setup`: it puts the akuaku binary on the
// user's PATH and reports which backend CLIs are installed. It is the onboarding
// bridge until Akuaku ships via Homebrew, where PATH is handled for you.
package setup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const filePerm = 0o644

// rename is a seam over os.Rename so the otherwise-unreachable atomic-write error
// path stays testable.
var rename = os.Rename

// Config is the environment `Run` inspects and mutates, injected so the logic is
// filesystem- and shell-agnostic under test.
type Config struct {
	BinDir   string                       // directory holding the akuaku binary
	Path     string                       // the current PATH value
	Profile  string                       // shell profile to edit when PATH needs fixing
	Backends []string                     // backend keys to check for, e.g. claude/codex/ollama
	LookPath func(string) (string, error) // seam over exec.LookPath
}

// Run ensures BinDir is on PATH — adding an export to the profile when it is not
// — and prints a report of the backend CLIs found on PATH.
func Run(cfg Config, out io.Writer) error {
	fmt.Fprintln(out, "Akuaku setup")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "PATH")
	if onPath(cfg.Path, cfg.BinDir) {
		fmt.Fprintf(out, "  ✓ %s is on your PATH — you're good\n", cfg.BinDir)
	} else {
		added, err := ensureExport(cfg.Profile, cfg.BinDir)
		if err != nil {
			return err
		}
		if added {
			fmt.Fprintf(out, "  ✗ %s was not on your PATH — added it to %s\n", cfg.BinDir, cfg.Profile)
		} else {
			fmt.Fprintf(out, "  ✗ %s is not active yet — already in %s\n", cfg.BinDir, cfg.Profile)
		}
		fmt.Fprintf(out, "    restart your shell or run:  source %s\n", cfg.Profile)
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Backends (needed to launch agents)")
	for _, name := range cfg.Backends {
		if _, err := cfg.LookPath(name); err == nil {
			fmt.Fprintf(out, "  ✓ %s\n", name)
		} else {
			fmt.Fprintf(out, "  ✗ %s — not found; install it to use `akuaku run %s`\n", name, name)
		}
	}
	return nil
}

// onPath reports whether dir is one of the PATH entries.
func onPath(path, dir string) bool {
	for _, entry := range filepath.SplitList(path) {
		if entry == dir {
			return true
		}
	}
	return false
}

// ensureExport appends a PATH export for binDir to profile unless it is already
// present, returning whether it made a change. The write is atomic, so a crash
// never corrupts the user's shell profile.
func ensureExport(profile, binDir string) (added bool, err error) {
	line := fmt.Sprintf(`export PATH="$PATH:%s"`, binDir)

	data, err := os.ReadFile(profile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if strings.Contains(string(data), line) {
		return false, nil
	}

	updated := append(data, []byte("\n# Added by `akuaku setup`\n"+line+"\n")...)
	tmp := profile + ".tmp"
	if err := os.WriteFile(tmp, updated, filePerm); err != nil {
		return false, err
	}
	if err := rename(tmp, profile); err != nil {
		return false, err
	}
	return true, nil
}

// ProfileFor returns the shell profile to edit for the given $SHELL, defaulting
// to ~/.profile for shells it does not specifically know.
func ProfileFor(shell, home string) string {
	switch {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zshrc")
	case strings.Contains(shell, "bash"):
		return filepath.Join(home, ".bashrc")
	default:
		return filepath.Join(home, ".profile")
	}
}
