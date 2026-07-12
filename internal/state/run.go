// Package state defines Akuaku's run state contract: the JSON files that any
// producer writes and the monitor reads. The state directory is the project's
// public interface, so this package is deliberately small and dependency-free.
package state

import "time"

// Status is the lifecycle state of a run.
type Status string

// The lifecycle statuses of a run. A run starts running and transitions exactly
// once to a terminal status.
const (
	StatusRunning Status = "running"
	StatusDone    Status = "done"
	StatusError   Status = "error"
)

// Run is a single agent run, serialized as one JSON file per run. EndedAt and
// ExitCode are pointers so they serialize as null while the run is still in
// progress, distinguishing "not finished" from a zero end time or exit code.
type Run struct {
	ID        string     `json:"id"`
	Backend   string     `json:"backend"`
	Name      string     `json:"name"`
	Status    Status     `json:"status"`
	Task      string     `json:"task"`
	Model     string     `json:"model,omitempty"`
	Source    string     `json:"source,omitempty"`
	PID       int        `json:"pid,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Tokens    int        `json:"tokens"`
	Cost      float64    `json:"cost"`
	ExitCode  *int       `json:"exit_code"`
	Error     string     `json:"error,omitempty"`
	Output    string     `json:"output,omitempty"`
}
