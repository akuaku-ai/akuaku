package discover_test

import (
	"testing"
	"time"

	"github.com/akuaku-ai/akuaku/internal/discover"
	"github.com/akuaku-ai/akuaku/internal/state"
)

func TestMatch_IdentifiesAgentsAndRejectsNoise(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		backend string
		ok      bool
	}{
		{"claude cli", []string{"claude"}, "claude", true},
		{"codex with path", []string{"/usr/local/bin/codex", "exec"}, "codex", true},
		{"ollama run", []string{"/opt/homebrew/bin/ollama", "run", "llama3.1"}, "ollama", true},
		{"ollama serve daemon", []string{"/opt/homebrew/bin/ollama", "serve"}, "", false},
		{"unrelated program", []string{"node", "server.js"}, "", false},
		{"claude desktop app", []string{"/Applications/Claude.app/Contents/MacOS/Claude"}, "", false},
		{"empty argv", nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			backend, ok := discover.Match(c.args)
			if backend != c.backend || ok != c.ok {
				t.Errorf("Match(%v) = (%q, %v), want (%q, %v)", c.args, backend, ok, c.backend, c.ok)
			}
		})
	}
}

func TestList_KeepsAgentCLIsAsRunningRuns(t *testing.T) {
	procs := []discover.Process{
		{PID: 100, Args: []string{"claude"}, Cwd: "/home/u/proj"},
		{PID: 101, Args: []string{"codex", "exec"}, Cwd: "/home/u/api"},
		{PID: 102, Args: []string{"/opt/homebrew/bin/ollama", "run", "llama3.1"}},
	}

	runs := discover.List(procs, 999)

	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3: %+v", len(runs), runs)
	}
	for _, run := range runs {
		if run.Status != state.StatusRunning {
			t.Errorf("%s: status = %q, want running", run.Backend, run.Status)
		}
		if run.Source != state.SourceProcess {
			t.Errorf("%s: source = %q, want %q", run.Backend, run.Source, state.SourceProcess)
		}
	}
	if got := []string{runs[0].Backend, runs[1].Backend, runs[2].Backend}; got[0] != "claude" || got[1] != "codex" || got[2] != "ollama" {
		t.Errorf("backends = %v", got)
	}
}

func TestList_ExcludesNonAgentsDaemonsAndSelf(t *testing.T) {
	procs := []discover.Process{
		{PID: 1, Args: []string{"node", "server.js"}},                              // not an agent
		{PID: 2, Args: []string{"/opt/homebrew/bin/ollama", "serve"}},              // daemon, not a run
		{PID: 3, Args: []string{"claude"}},                                         // the monitor's own PID
		{PID: 4, Args: []string{"/Applications/Claude.app/Contents/MacOS/Claude"}}, // the desktop app, not the CLI
		{PID: 5, Args: nil}, // no command line
	}

	if runs := discover.List(procs, 3); len(runs) != 0 {
		t.Fatalf("got %d runs, want 0: %+v", len(runs), runs)
	}
}

func TestList_IdentifiesByArgv0EvenWhenExeIsVersionStamped(t *testing.T) {
	// Claude Code's executable is a version-stamped path, but argv[0] is "claude".
	procs := []discover.Process{{PID: 7, Args: []string{"claude", "--resume", "abc"}, Cwd: "/home/u/dev"}}

	runs := discover.List(procs, 0)

	if len(runs) != 1 || runs[0].Backend != "claude" {
		t.Fatalf("argv[0] identity failed: %+v", runs)
	}
}

func TestList_MapsProcessFields(t *testing.T) {
	started := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	procs := []discover.Process{
		{PID: 42, Args: []string{"claude"}, Cwd: "/home/u/dev-akuaku", StartedAt: started},
	}

	run := discover.List(procs, 0)[0]

	if run.ID != "proc-42" {
		t.Errorf("id = %q, want proc-42", run.ID)
	}
	if run.PID != 42 {
		t.Errorf("pid = %d, want 42", run.PID)
	}
	if run.Dir != "/home/u/dev-akuaku" {
		t.Errorf("dir = %q", run.Dir)
	}
	if run.Name != "dev-akuaku" {
		t.Errorf("name = %q, want the cwd basename", run.Name)
	}
	if !run.StartedAt.Equal(started) {
		t.Errorf("startedAt = %v, want %v", run.StartedAt, started)
	}
}

func TestList_OllamaModelParsedFromArgs(t *testing.T) {
	run := discover.List([]discover.Process{
		{PID: 1, Args: []string{"ollama", "run", "llama3.1", "--verbose"}},
	}, 0)[0]

	if run.Model != "llama3.1" {
		t.Errorf("model = %q, want llama3.1", run.Model)
	}
}

func TestList_OllamaRunWithoutModelHasNoModel(t *testing.T) {
	run := discover.List([]discover.Process{
		{PID: 1, Args: []string{"ollama", "run"}},
	}, 0)[0]

	if run.Model != "" {
		t.Errorf("model = %q, want empty", run.Model)
	}
}

func TestList_NameFallsBackToBackendWhenNoCwd(t *testing.T) {
	run := discover.List([]discover.Process{
		{PID: 1, Args: []string{"codex"}},
	}, 0)[0]

	if run.Name != "codex session" {
		t.Errorf("name = %q, want %q", run.Name, "codex session")
	}
}
