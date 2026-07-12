package backend

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestGet_KnownBackends(t *testing.T) {
	for _, key := range []string{"claude", "codex", "ollama"} {
		b, err := Get(key)
		if err != nil {
			t.Fatalf("Get(%q) error: %v", key, err)
		}
		if b.Key() != key {
			t.Errorf("Key() = %q, want %q", b.Key(), key)
		}
	}
}

func TestGet_UnknownBackendErrors(t *testing.T) {
	if _, err := Get("nope"); err == nil {
		t.Fatal("expected an error for an unknown backend")
	}
}

func TestKeys_SortedAndComplete(t *testing.T) {
	if got := Keys(); !slices.Equal(got, []string{"claude", "codex", "ollama"}) {
		t.Errorf("Keys() = %v", got)
	}
}

func TestClaudeCommand_AppliesModel(t *testing.T) {
	b, _ := Get("claude")
	name, args := b.Command("do it", "opus")
	if name != "claude" {
		t.Errorf("name = %q, want claude", name)
	}
	for _, want := range []string{"-p", "do it", "--output-format", "json", "--model", "opus"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

func TestClaudeCommand_OmitsEmptyModel(t *testing.T) {
	b, _ := Get("claude")
	if _, args := b.Command("t", ""); slices.Contains(args, "--model") {
		t.Errorf("should not add --model when empty: %v", args)
	}
}

func TestClaudeParse_FixtureOutput(t *testing.T) {
	b, _ := Get("claude")
	out := b.Parse(readFixture(t, "claude.json"), nil)
	if out.Text != "ok" {
		t.Errorf("text = %q, want %q", out.Text, "ok")
	}
	if out.Tokens != 6 {
		t.Errorf("tokens = %d, want 6 (2 in + 4 out)", out.Tokens)
	}
	if out.Cost < 0.1044 || out.Cost > 0.1045 {
		t.Errorf("cost = %v, want ~0.10442", out.Cost)
	}
}

func TestClaudeParse_GarbageDegradesToZero(t *testing.T) {
	b, _ := Get("claude")
	out := b.Parse([]byte("not json"), nil)
	if out.Tokens != 0 || out.Cost != 0 || out.Text != "" {
		t.Errorf("garbage should yield an empty Output, got %+v", out)
	}
}

func TestCodexCommand_AppliesModel(t *testing.T) {
	b, _ := Get("codex")
	name, args := b.Command("do it", "o3")
	if name != "codex" {
		t.Errorf("name = %q, want codex", name)
	}
	for _, want := range []string{"exec", "--json", "--skip-git-repo-check", "-m", "o3", "do it"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

func TestCodexCommand_OmitsEmptyModel(t *testing.T) {
	b, _ := Get("codex")
	if _, args := b.Command("t", ""); slices.Contains(args, "-m") {
		t.Errorf("should not add -m when empty: %v", args)
	}
}

func TestCodexParse_FixtureOutput(t *testing.T) {
	b, _ := Get("codex")
	out := b.Parse(readFixture(t, "codex.jsonl"), nil)
	if out.Text != "ok" {
		t.Errorf("text = %q, want %q", out.Text, "ok")
	}
	if out.Tokens != 26569 {
		t.Errorf("tokens = %d, want 26569 (26533 in + 36 out)", out.Tokens)
	}
	if out.Cost != 0 {
		t.Errorf("cost = %v, want 0", out.Cost)
	}
}

func TestCodexParse_IgnoresJunkAndMissingUsage(t *testing.T) {
	b, _ := Get("codex")
	input := []byte("not json\n{\"type\":\"turn.started\"}\n\n")
	out := b.Parse(input, nil)
	if out.Tokens != 0 || out.Text != "" {
		t.Errorf("junk should yield an empty Output, got %+v", out)
	}
}

func TestOllamaCommand_UsesPositionalModel(t *testing.T) {
	b, _ := Get("ollama")
	name, args := b.Command("do it", "llama3.1")
	if name != "ollama" {
		t.Errorf("name = %q, want ollama", name)
	}
	if !slices.Equal(args, []string{"run", "llama3.1", "do it", "--verbose"}) {
		t.Errorf("args = %v", args)
	}
}

func TestOllamaParse_FixtureOutput(t *testing.T) {
	b, _ := Get("ollama")
	// Ollama writes the answer to stdout and its --verbose stats to stderr.
	out := b.Parse([]byte("  the answer\n"), readFixture(t, "ollama_verbose.txt"))
	if out.Text != "the answer" {
		t.Errorf("text = %q, want %q", out.Text, "the answer")
	}
	if out.Tokens != 17 {
		t.Errorf("tokens = %d, want 17 (15 prompt + 2 eval)", out.Tokens)
	}
	if out.Cost != 0 {
		t.Errorf("cost = %v, want 0", out.Cost)
	}
}

func TestOllamaParse_NoMatchDegradesToZero(t *testing.T) {
	b, _ := Get("ollama")
	out := b.Parse(nil, []byte("no stats here"))
	if out.Tokens != 0 || out.Text != "" {
		t.Errorf("no match should yield an empty Output, got %+v", out)
	}
}
