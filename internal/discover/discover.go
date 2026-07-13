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

// List turns the agent CLI processes among procs into running runs. It skips
// selfPID (the monitor itself), processes with no command line, non-agent
// programs, and the `ollama serve` daemon, which is not an agent run.
func List(procs []Process, selfPID int) []state.Run {
	var runs []state.Run
	for _, p := range procs {
		if p.PID == selfPID || len(p.Args) == 0 {
			continue
		}
		backend, ok := agents[filepath.Base(p.Args[0])]
		if !ok {
			continue
		}

		model := ""
		if backend == "ollama" {
			i := argIndex(p.Args, "run")
			if i < 0 {
				continue // `ollama serve` and friends are not runs
			}
			if i+1 < len(p.Args) {
				model = p.Args[i+1]
			}
		}

		runs = append(runs, state.Run{
			ID:        fmt.Sprintf("proc-%d", p.PID),
			Backend:   backend,
			Name:      name(backend, p.Cwd),
			Status:    state.StatusRunning,
			Model:     model,
			Source:    state.SourceProcess,
			PID:       p.PID,
			Dir:       p.Cwd,
			StartedAt: p.StartedAt,
		})
	}
	return runs
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
