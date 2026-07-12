// Package cli parses Akuaku's command line and dispatches to the monitor or the
// launcher. The side effects (running the TUI, launching a run) are injected so
// the dispatch logic is fully testable.
package cli

import (
	"fmt"
	"io"

	"github.com/akuaku-ai/akuaku/internal/launcher"
	"github.com/akuaku-ai/akuaku/internal/state"
)

// Deps are the injectable behaviors the CLI drives.
type Deps struct {
	Monitor func() error
	Launch  func(launcher.Options) error
	Out     io.Writer
	Err     io.Writer
}

const usage = `akuaku — monitor and launch AI agents from the terminal

usage:
  akuaku                                 start the monitor
  akuaku run <backend> <task> [flags]    launch an agent

flags:
  -m, --model <model>   model to use
  -n, --name  <name>    display name for the run

backends: claude, codex, ollama`

// Run parses args and dispatches, returning a process exit code.
func Run(args []string, deps Deps) int {
	if len(args) == 0 {
		if err := deps.Monitor(); err != nil {
			fmt.Fprintln(deps.Err, "akuaku:", err)
			return 1
		}
		return 0
	}

	switch args[0] {
	case "run":
		return runCommand(args[1:], deps)
	case "-h", "--help", "help":
		fmt.Fprintln(deps.Out, usage)
		return 0
	default:
		fmt.Fprintf(deps.Err, "akuaku: unknown command %q\n", args[0])
		return 2
	}
}

func runCommand(args []string, deps Deps) int {
	opts, err := parseRunArgs(args)
	if err != nil {
		fmt.Fprintln(deps.Err, "akuaku:", err)
		return 2
	}
	opts.Dir = state.Dir()
	if err := deps.Launch(opts); err != nil {
		fmt.Fprintln(deps.Err, "akuaku:", err)
		return 1
	}
	return 0
}

// parseRunArgs extracts the backend, task, and optional model/name from the
// arguments to `run`. Flags may appear in any position.
func parseRunArgs(args []string) (launcher.Options, error) {
	var opts launcher.Options
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--model", "-m", "--name", "-n":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("flag %s needs a value", arg)
			}
			if arg == "--model" || arg == "-m" {
				opts.Model = args[i]
			} else {
				opts.Name = args[i]
			}
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) < 2 {
		return opts, fmt.Errorf("usage: akuaku run <backend> <task> [--model m] [--name n]")
	}
	opts.Backend = positional[0]
	opts.Task = positional[1]
	return opts, nil
}
