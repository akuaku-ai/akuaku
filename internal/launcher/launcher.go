// Package launcher runs agent backends and records each run's lifecycle to the
// state directory. The monitor never coordinates with it — the state files are
// the only channel between them.
package launcher

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/akuaku-ai/akuaku/internal/backend"
	"github.com/akuaku-ai/akuaku/internal/state"
)

// commandRunner executes name with args, invoking onStart with the child PID
// once the process is running, and returns captured output, the process exit
// code, and a start error (nil once the process actually ran).
type commandRunner func(name string, args []string, onStart func(pid int)) (stdout, stderr []byte, exitCode int, err error)

// Options configures a single run.
type Options struct {
	Backend string
	Task    string
	Model   string
	Name    string
	Dir     string
}

// Launcher records agent runs. Its dependencies are injectable so the lifecycle
// can be tested without real subprocesses, clock, randomness, or terminal.
type Launcher struct {
	run    commandRunner
	write  func(dir string, run state.Run) error
	now    func() time.Time
	suffix func() (string, error)
	out    io.Writer
}

// New returns a Launcher wired to real subprocess execution, the system clock,
// and cryptographic randomness, reporting progress and the answer to out.
func New(out io.Writer) *Launcher {
	return &Launcher{
		run:    execRun,
		write:  state.Write,
		now:    time.Now,
		suffix: func() (string, error) { return state.RandomSuffix(rand.Reader) },
		out:    out,
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
	fmt.Fprintf(l.out, "running %s…\n", opts.Backend)

	// Record the running run once the process starts, so its PID is on disk and
	// the monitor can kill it. A start failure never calls onStart and falls
	// through to the terminal write below.
	name, args := b.Command(opts.Task, opts.Model)
	var writeErr error
	stdout, stderr, exitCode, runErr := l.run(name, args, func(pid int) {
		run.PID = pid
		writeErr = l.write(opts.Dir, run)
	})
	if writeErr != nil {
		return writeErr
	}

	ended := l.now()
	run.EndedAt = &ended
	run.ExitCode = &exitCode
	out := b.Parse(stdout, stderr)
	run.Tokens, run.Cost, run.Output = out.Tokens, out.Cost, out.Text
	if runErr != nil || exitCode != 0 {
		run.Status = state.StatusError
		run.Error = errorMessage(runErr, stderr)
	} else {
		run.Status = state.StatusDone
	}

	printResult(l.out, run)
	return l.write(opts.Dir, run)
}

// printResult reports a finished run to the terminal: the answer on success, or
// the failure reason on error.
func printResult(w io.Writer, run state.Run) {
	if run.Status == state.StatusError {
		fmt.Fprintf(w, "\n─── %s · error ───\n%s\n", run.Backend, run.Error)
		return
	}
	fmt.Fprintf(w, "\n─── %s · done · %d tok · $%.2f ───\n", run.Backend, run.Tokens, run.Cost)
	if run.Output != "" {
		fmt.Fprintln(w, run.Output)
	}
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

// execRun runs a real subprocess, capturing stdout and stderr and reporting the
// child PID through onStart once it is running. A non-zero exit is reported
// through exitCode with a nil error; only a failure to start returns an error
// (and never calls onStart).
func execRun(name string, args []string, onStart func(pid int)) ([]byte, []byte, int, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		code, e := classifyExit(err)
		return nil, nil, code, e
	}
	onStart(cmd.Process.Pid)

	code, e := classifyExit(cmd.Wait())
	return stdout.Bytes(), stderr.Bytes(), code, e
}

// classifyExit maps a start or wait error to an exit code: 0 on success, the
// process's own code for a normal non-zero exit (including -1 when signaled), or
// -1 for a failure to start.
func classifyExit(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return -1, err
}
