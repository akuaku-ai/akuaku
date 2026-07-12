// Command akuaku is a terminal UI to monitor and launch AI agents.
//
// With no arguments it starts the monitor; `akuaku run <backend> <task>`
// launches an agent. This entrypoint only wires the pieces together; all
// behavior lives in testable internal packages.
package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/akuaku-ai/akuaku/internal/cli"
	"github.com/akuaku-ai/akuaku/internal/launcher"
	"github.com/akuaku-ai/akuaku/internal/tui"
)

func main() {
	deps := cli.Deps{
		Monitor: runMonitor,
		Launch:  launcher.New().Run,
		Out:     os.Stdout,
		Err:     os.Stderr,
	}
	os.Exit(cli.Run(os.Args[1:], deps))
}

func runMonitor() error {
	_, err := tea.NewProgram(tui.New(), tea.WithAltScreen()).Run()
	return err
}
