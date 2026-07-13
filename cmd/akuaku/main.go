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
	"github.com/shirou/gopsutil/v4/process"

	"github.com/akuaku-ai/akuaku/internal/backend"
	"github.com/akuaku-ai/akuaku/internal/cli"
	"github.com/akuaku-ai/akuaku/internal/discover"
	"github.com/akuaku-ai/akuaku/internal/hook"
	"github.com/akuaku-ai/akuaku/internal/launcher"
	"github.com/akuaku-ai/akuaku/internal/setup"
	"github.com/akuaku-ai/akuaku/internal/state"
	"github.com/akuaku-ai/akuaku/internal/tui"
	"github.com/akuaku-ai/akuaku/internal/updater"
)

// version is the build's reported version, overridden at link time via
// -ldflags "-X main.version=...". It stays "dev" for a plain `go build`.
var version = "dev"

func main() {
	deps := cli.Deps{
		Monitor: runMonitor,
		Version: version,
		Launch:  launcher.New(os.Stdout).Run,
		Hook: func(event string, r io.Reader) error {
			return hook.Handle(event, r, state.Dir(), time.Now())
		},
		HookInstall: installHooks,
		Setup:       runSetup,
		Update:      runUpdate,
		In:          os.Stdin,
		Out:         os.Stdout,
		Err:         os.Stderr,
	}
	tui.SetProcessSource(scanProcesses)
	os.Exit(cli.Run(os.Args[1:], deps))
}

// scanProcesses snapshots the OS process table with gopsutil and hands the agent
// processes to the discovery logic, so the monitor surfaces agent sessions
// started outside Akuaku. It runs on every refresh tick, so it pays for the
// expensive fields (working directory, start time) only for the few processes
// discover.Match recognizes as agents, not the hundreds it skips. A process that
// vanished or is unreadable is dropped rather than failing the scan.
func scanProcesses() []state.Run {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	var agents []discover.Process
	for _, p := range procs {
		// argv identifies the program; a CLI's argv[0] is its command name even
		// when the executable is a version-stamped path.
		args, err := p.CmdlineSlice()
		if err != nil {
			continue
		}
		if _, ok := discover.Match(args); !ok {
			continue
		}
		cwd, _ := p.Cwd()
		var started time.Time
		if ms, err := p.CreateTime(); err == nil {
			started = time.UnixMilli(ms)
		}
		agents = append(agents, discover.Process{
			PID:       int(p.Pid),
			Args:      args,
			Cwd:       cwd,
			StartedAt: started,
		})
	}
	return discover.List(agents, os.Getpid())
}

// runUpdate reinstalls Akuaku from source. GOPROXY=direct bypasses the module
// proxy cache so the update always reflects the newest published build.
func runUpdate() error {
	return updater.Run(func() ([]byte, error) {
		cmd := exec.Command("go", "install", updater.Module)
		cmd.Env = append(os.Environ(), "GOPROXY=direct")
		return cmd.CombinedOutput()
	}, os.Stdout)
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
