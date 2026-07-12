// Package launcher runs agent backends and records each run's lifecycle to the
// state directory. The monitor never coordinates with it — the state files are
// the only channel between them.
package launcher

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/akuaku-ai/akuaku/internal/backend"
	"github.com/akuaku-ai/akuaku/internal/state"
)

// commandRunner executes name with args and returns captured output, the process
// exit code, and a start error (nil once the process actually ran).
type commandRunner func(name string, args []string) (stdout, stderr []byte, exitCode int, err error)

// Options configures a single run.
type Options struct {
	Backend string
	Task    string
	Model   string
	Name    string
	Dir     string
}

// Launcher records agent runs. Its dependencies are injectable so the lifecycle
// can be tested without real subprocesses, clock, or randomness.
type Launcher struct {
	run    commandRunner
	write  func(dir string, run state.Run) error
	now    func() time.Time
	suffix func() (string, error)
}

// New returns a Launcher wired to real subprocess execution, the system clock,
// and cryptographic randomness.
func New() *Launcher {
	return &Launcher{
		run:    execRun,
		write:  state.Write,
		now:    time.Now,
		suffix: func() (string, error) { return state.RandomSuffix(rand.Reader) },
	}
}

// Run launches opts.Backend and records its lifecycle to opts.Dir. It returns an
// error only when a run cannot be recorded (unknown backend, randomness or write
// failure); a failing subprocess is recorded as an error run, not returned.
func (l *Launcher) Run(opts Options) error {
	b, err := backend.Get(opts.Backend)
	if err != nil {
		return err
	}
	suffix, err := l.suffix()
	if err != nil {
		return err
	}

	started := l.now()
	run := state.Run{
		ID:        state.NewID(opts.Backend, started, suffix),
		Backend:   opts.Backend,
		Name:      displayName(opts),
		Status:    state.StatusRunning,
		Task:      opts.Task,
		Model:     opts.Model,
		StartedAt: started,
	}
	if err := l.write(opts.Dir, run); err != nil {
		return err
	}

	name, args := b.Command(opts.Task, opts.Model)
	stdout, stderr, exitCode, runErr := l.run(name, args)

	ended := l.now()
	run.EndedAt = &ended
	run.ExitCode = &exitCode
	run.Tokens, run.Cost = b.Parse(stdout, stderr)
	if runErr != nil || exitCode != 0 {
		run.Status = state.StatusError
		run.Error = errorMessage(runErr, stderr)
	} else {
		run.Status = state.StatusDone
	}
	return l.write(opts.Dir, run)
}

// displayName is the label shown in the monitor: an explicit name, or the task.
func displayName(opts Options) string {
	if opts.Name != "" {
		return opts.Name
	}
	return opts.Task
}

// errorMessage summarizes why a run failed.
func errorMessage(runErr error, stderr []byte) string {
	if runErr != nil {
		return runErr.Error()
	}
	if msg := strings.TrimSpace(string(stderr)); msg != "" {
		return msg
	}
	return "process exited with a non-zero status"
}

// execRun runs a real subprocess, capturing stdout and stderr. A non-zero exit
// is reported through exitCode with a nil error; only a failure to start the
// process returns an error.
func execRun(name string, args []string) ([]byte, []byte, int, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			err = nil
		} else {
			exitCode = -1
		}
	}
	return stdout.Bytes(), stderr.Bytes(), exitCode, err
}
