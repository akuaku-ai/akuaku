package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const (
	envStateDir     = "AKUAKU_STATE_DIR"
	defaultStateDir = "state"
	dirPerm         = 0o755
	filePerm        = 0o644
)

// marshal is a seam over json.MarshalIndent so tests can exercise the (otherwise
// unreachable) marshal error path.
var marshal = func(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

// Dir returns the state directory: AKUAKU_STATE_DIR when set, otherwise
// ~/.akuaku/state, falling back to a local "state" directory when the home
// directory cannot be determined. An absolute default lets the monitor and the
// launcher agree on the same directory regardless of the working directory.
func Dir() string {
	if d := os.Getenv(envStateDir); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultStateDir
	}
	return filepath.Join(home, ".akuaku", "state")
}

// Write serializes run to dir/<id>.json through a temporary file and an atomic
// rename, so a concurrent reader never observes a partially written file.
func Write(dir string, run Run) error {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	data, err := marshal(run)
	if err != nil {
		return err
	}
	final := filepath.Join(dir, run.ID+".json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// ReadDir returns every run in dir. Files that cannot be read or parsed are
// skipped, and a missing directory yields no runs and no error.
func ReadDir(dir string) ([]Run, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var runs []Run
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var run Run
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// Read returns the run stored as <id>.json in dir. found is false when no such
// run exists; a parse or read failure returns an error.
func Read(dir, id string) (run Run, found bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, err
	}
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, false, err
	}
	return run, true, nil
}
