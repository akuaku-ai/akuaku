// Command akuaku is a terminal UI to monitor and launch AI agents.
//
// With no arguments it starts the monitor; `akuaku run <backend> <task>`
// launches an agent; `akuaku hook install` reflects Claude sessions started
// elsewhere. This entrypoint only wires the pieces together; all behavior lives
// in testable internal packages.
package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akuaku-ai/akuaku/internal/backend"
	"github.com/akuaku-ai/akuaku/internal/cli"
	"github.com/akuaku-ai/akuaku/internal/hook"
	"github.com/akuaku-ai/akuaku/internal/launcher"
	"github.com/akuaku-ai/akuaku/internal/setup"
	"github.com/akuaku-ai/akuaku/internal/state"
	"github.com/akuaku-ai/akuaku/internal/tui"
)

func main() {
	deps := cli.Deps{
		Monitor: runMonitor,
		Launch:  launcher.New().Run,
		Hook: func(event string, r io.Reader) error {
			return hook.Handle(event, r, state.Dir(), time.Now())
		},
		HookInstall: installHooks,
		Setup:       runSetup,
		In:          os.Stdin,
		Out:         os.Stdout,
		Err:         os.Stderr,
	}
	os.Exit(cli.Run(os.Args[1:], deps))
}

func runMonitor() error {
	_, err := tea.NewProgram(tui.New(), tea.WithAltScreen()).Run()
	return err
}

// installHooks merges Akuaku's hooks into the user's Claude Code settings,
// invoking this very binary so the command works regardless of PATH.
func installHooks() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	settings := filepath.Join(home, ".claude", "settings.json")
	exe, err := os.Executable()
	if err != nil {
		exe = "akuaku"
	}
	return hook.Install(settings, exe)
}

// runSetup puts this binary's directory on the user's PATH and reports which
// backend CLIs are installed.
func runSetup() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return setup.Run(setup.Config{
		BinDir:   filepath.Dir(exe),
		Path:     os.Getenv("PATH"),
		Profile:  setup.ProfileFor(os.Getenv("SHELL"), home),
		Backends: backend.Keys(),
		LookPath: exec.LookPath,
	}, os.Stdout)
}
