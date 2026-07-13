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
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/akuaku-ai/akuaku/internal/backend"
	"github.com/akuaku-ai/akuaku/internal/cli"
	"github.com/akuaku-ai/akuaku/internal/demo"
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
		Demo:        runDemo,
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

// runDemo shows the monitor against a simulated fleet in a throwaway state
// directory, so anyone can see Akuaku alive with no agents of their own. A
// background writer advances the demo frames while the monitor reads them, and
// the directory is removed on exit.
func runDemo() error {
	dir, err := os.MkdirTemp("", "akuaku-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.Setenv("AKUAKU_STATE_DIR", dir); err != nil {
		return err
	}
	tui.SetProcessSource(func() []state.Run { return nil }) // show only the demo's own agents

	cwd, _ := os.Getwd()
	base := time.Now()
	// Seed the full first frame before the monitor opens, so it shows the whole
	// fleet from its first render instead of catching a half-written directory.
	writeDemoFrame(dir, cwd, base, 0)
	stop := make(chan struct{})
	go advanceDemo(dir, cwd, base, stop)

	err = runMonitor()
	close(stop)
	return err
}

// advanceDemo writes one demo frame per second, from tick 1, until stop closes.
func advanceDemo(dir, cwd string, base time.Time, stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for tick := 1; ; tick++ {
		select {
		case <-stop:
			return
		case <-ticker.C:
			writeDemoFrame(dir, cwd, base, tick)
		}
	}
}

// writeDemoFrame writes the agents present at tick and removes any that have left
// the frame, so a launched-and-gone agent does not linger.
func writeDemoFrame(dir, cwd string, base time.Time, tick int) {
	present := map[string]bool{}
	for _, run := range demo.Frame(tick, base) {
		run.Dir = cwd
		present[run.ID] = true
		_ = state.Write(dir, run)
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if strings.HasSuffix(entry.Name(), ".json") && !present[id] {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
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
