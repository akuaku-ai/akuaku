// Package discover surfaces agent sessions that are already running when the
// monitor opens — sessions no hook or `akuaku run` recorded. It scans a snapshot
// of the process table (gathered by the binary) and maps recognized agent CLIs
// to running runs, so they appear in the monitor alongside everything else.
//
// A discovered run carries only what the OS knows: backend, PID, working
// directory, and start time. It has no task or usage, matching the honest gaps
// of a reflected session.
package discover

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/akuaku-ai/akuaku/internal/state"
)

// Process is a platform-neutral snapshot of one OS process. The binary fills it
// from gopsutil; tests use plain fixtures, so List needs no real processes.
//
// Args is the command line as argv. Args[0] identifies the program — a CLI's
// argv[0] is its command name ("claude") even when the executable is a
// version-stamped path, so it, not the executable base name, is what we match.
type Process struct {
	PID       int
	Args      []string
	Cwd       string // working directory, "" if unknown
	StartedAt time.Time
}

// agents maps a recognized CLI command to its backend label. The match is exact
// and case-sensitive, so the "Claude" desktop app (argv[0] ".../Claude") is not
// mistaken for the "claude" CLI.
var agents = map[string]string{
	"claude": "claude",
	"codex":  "codex",
	"ollama": "ollama",
}

// List turns the agent CLI processes among procs into running runs, skipping
// selfPID (the monitor itself) and anything Match rejects.
func List(procs []Process, selfPID int) []state.Run {
	var runs []state.Run
	for _, p := range procs {
		if p.PID == selfPID {
			continue
		}
		backend, ok := Match(p.Args)
		if !ok {
			continue
		}
		runs = append(runs, state.Run{
			ID:        fmt.Sprintf("proc-%d", p.PID),
			Backend:   backend,
			Name:      name(backend, p.Cwd),
			Status:    state.StatusRunning,
			Model:     ollamaModel(backend, p.Args),
			Source:    state.SourceProcess,
			PID:       p.PID,
			Dir:       p.Cwd,
			StartedAt: p.StartedAt,
		})
	}
	return runs
}

// Match reports the backend of a process from its argv, or ok=false when it is
// not an agent run. It identifies by the base name of argv[0] (a CLI's command
// name even when the executable is a version-stamped path) and rejects the
// `ollama serve` daemon. It is exported so a scanner can cheaply skip non-agents
// before paying to read their working directory.
func Match(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	backend, ok := agents[filepath.Base(args[0])]
	if !ok {
		return "", false
	}
	if backend == "ollama" && argIndex(args, "run") < 0 {
		return "", false
	}
	return backend, true
}

// ollamaModel returns the model named by an `ollama run <model>` command, or ""
// for other backends or an `ollama run` with no model.
func ollamaModel(backend string, args []string) string {
	if backend != "ollama" {
		return ""
	}
	if i := argIndex(args, "run"); i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

// name labels a discovered run with its working directory's base name, so
// sessions in different folders are distinguishable, falling back to the backend
// when the directory is unknown.
func name(backend, cwd string) string {
	if cwd == "" {
		return backend + " session"
	}
	return filepath.Base(cwd)
}

// argIndex returns the position of want in args, or -1 if absent.
func argIndex(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}
