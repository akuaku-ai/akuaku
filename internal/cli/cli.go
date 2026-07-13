// Package cli parses Akuaku's command line and dispatches to the monitor or the
// launcher. The side effects (running the TUI, launching a run) are injected so
// the dispatch logic is fully testable.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/akuaku-ai/akuaku/internal/brand"
	"github.com/akuaku-ai/akuaku/internal/launcher"
	"github.com/akuaku-ai/akuaku/internal/state"
)

// Deps are the injectable behaviors the CLI drives.
type Deps struct {
	Monitor     func() error
	Launch      func(launcher.Options) error
	Hook        func(event string, r io.Reader) error
	HookInstall func() error
	Setup       func() error
	Update      func() error
	In          io.Reader
	Out         io.Writer
	Err         io.Writer
	Version     string
}

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
	case "hook":
		return hookCommand(args[1:], deps)
	case "setup":
		if err := deps.Setup(); err != nil {
			fmt.Fprintln(deps.Err, "akuaku:", err)
			return 1
		}
		return 0
	case "update":
		if err := deps.Update(); err != nil {
			fmt.Fprintln(deps.Err, "akuaku:", err)
			return 1
		}
		return 0
	case "-h", "--help", "help":
		fmt.Fprint(deps.Out, helpText(deps.Version))
		return 0
	case "-v", "--version", "version":
		fmt.Fprintf(deps.Out, "akuaku %s\n", deps.Version)
		return 0
	default:
		fmt.Fprintf(deps.Err, "akuaku: unknown command %q\n", args[0])
		if s := suggest(args[0]); s != "" {
			fmt.Fprintf(deps.Err, "did you mean %q?\n", s)
		}
		fmt.Fprintln(deps.Err, "run 'akuaku help' to see available commands")
		return 2
	}
}

// knownCommands are the verbs `suggest` matches a typo against.
var knownCommands = []string{"run", "hook", "install", "setup", "update", "version", "help"}

// suggest returns the known command closest to cmd, or "" when nothing is within
// a two-edit distance — so a genuine typo gets a hint but noise does not.
func suggest(cmd string) string {
	best, bestDist := "", 3
	for _, known := range knownCommands {
		if d := levenshtein(cmd, known); d < bestDist {
			best, bestDist = known, d
		}
	}
	return best
}

// levenshtein is the edit distance between a and b: the fewest single-character
// insertions, deletions, or substitutions that turn one into the other.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// The help palette: aqua headings echo the monitor's accent, command names are
// bold, and descriptions are dimmed. Colors collapse to plain text off a TTY.
var (
	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(brand.Accent)
	nameStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle     = lipgloss.NewStyle().Faint(true)
)

const tagline = "monitor & launch AI agents — with the CLIs you already log into"

// row is one name/description pair in a help section.
type row struct{ name, desc string }

var (
	helpCommands = []row{
		{"(no args)", "open the live dashboard"},
		{"run <backend> <task>", "launch an agent and print its answer"},
		{"hook install", "reflect Claude sessions from other terminals"},
		{"setup", "add akuaku to your PATH & check backends"},
		{"update", "upgrade to the latest version"},
		{"version", "print the akuaku version"},
		{"help", "show this help"},
	}
	helpExamples = []row{
		{"akuaku", "open the monitor"},
		{`akuaku run claude "refactor auth.go"`, "ask Claude, see the answer"},
		{`akuaku run ollama "hola" -m llama3.1`, "run a local model"},
		{"akuaku hook install", "surface every Claude session"},
	}
	helpFlags = []row{
		{"-m, --model <model>", "model to use (required for ollama)"},
		{"-n, --name  <name>", "label the run in the monitor"},
	}
)

// helpText renders the branded help screen: the logo, then aligned sections.
func helpText(version string) string {
	var b strings.Builder
	for _, line := range brand.Header(tagline) {
		fmt.Fprintf(&b, "  %s\n", line)
	}
	b.WriteByte('\n')

	fmt.Fprintf(&b, "%s\n  akuaku [command] [flags]\n\n", headingStyle.Render("USAGE"))
	writeSection(&b, "COMMANDS", helpCommands)
	fmt.Fprintf(&b, "%s\n  claude · codex · ollama\n\n", headingStyle.Render("BACKENDS"))
	writeSection(&b, "EXAMPLES", helpExamples)
	writeSection(&b, "FLAGS", helpFlags)

	footer := "no API keys · your logins"
	if version != "" {
		footer += " · " + version
	}
	footer += " · https://github.com/akuaku-ai/akuaku"
	fmt.Fprintf(&b, "  %s\n", dimStyle.Render(footer))
	return b.String()
}

// writeSection prints a heading and its rows with the name column aligned, so
// descriptions line up regardless of name length.
func writeSection(b *strings.Builder, title string, rows []row) {
	width := 0
	for _, r := range rows {
		if len(r.name) > width {
			width = len(r.name)
		}
	}
	fmt.Fprintf(b, "%s\n", headingStyle.Render(title))
	for _, r := range rows {
		pad := strings.Repeat(" ", width-len(r.name))
		fmt.Fprintf(b, "  %s%s   %s\n", nameStyle.Render(r.name), pad, dimStyle.Render(r.desc))
	}
	b.WriteByte('\n')
}

// hookCommand dispatches the `hook` subcommands. `hook install` wires Akuaku into
// Claude Code's settings; `hook <event>` reflects a session event read from stdin
// and always exits 0, since a hook must never disrupt the host Claude session.
func hookCommand(args []string, deps Deps) int {
	if len(args) >= 1 && args[0] == "install" {
		if err := deps.HookInstall(); err != nil {
			fmt.Fprintln(deps.Err, "akuaku:", err)
			return 1
		}
		fmt.Fprintln(deps.Out, "akuaku: hooks installed; new Claude sessions will appear in the monitor")
		return 0
	}
	if len(args) < 1 {
		fmt.Fprintln(deps.Err, "akuaku: hook needs an event name")
		return 2
	}
	if err := deps.Hook(args[0], deps.In); err != nil {
		fmt.Fprintln(deps.Err, "akuaku:", err)
	}
	return 0
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
