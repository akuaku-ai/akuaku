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

func TestClaudeParse_FixtureTokensAndCost(t *testing.T) {
	b, _ := Get("claude")
	tokens, cost := b.Parse(readFixture(t, "claude.json"), nil)
	if tokens != 6 {
		t.Errorf("tokens = %d, want 6 (2 in + 4 out)", tokens)
	}
	if cost < 0.1044 || cost > 0.1045 {
		t.Errorf("cost = %v, want ~0.10442", cost)
	}
}

func TestClaudeParse_GarbageDegradesToZero(t *testing.T) {
	b, _ := Get("claude")
	if tokens, cost := b.Parse([]byte("not json"), nil); tokens != 0 || cost != 0 {
		t.Errorf("garbage should yield 0/0, got %d/%v", tokens, cost)
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

func TestCodexParse_FixtureTokens(t *testing.T) {
	b, _ := Get("codex")
	tokens, cost := b.Parse(readFixture(t, "codex.jsonl"), nil)
	if tokens != 26569 {
		t.Errorf("tokens = %d, want 26569 (26533 in + 36 out)", tokens)
	}
	if cost != 0 {
		t.Errorf("cost = %v, want 0", cost)
	}
}

func TestCodexParse_IgnoresJunkAndMissingUsage(t *testing.T) {
	b, _ := Get("codex")
	input := []byte("not json\n{\"type\":\"turn.started\"}\n\n")
	if tokens, _ := b.Parse(input, nil); tokens != 0 {
		t.Errorf("tokens = %d, want 0", tokens)
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

func TestOllamaParse_FixtureTokens(t *testing.T) {
	b, _ := Get("ollama")
	tokens, cost := b.Parse(nil, readFixture(t, "ollama_verbose.txt"))
	if tokens != 17 {
		t.Errorf("tokens = %d, want 17 (15 prompt + 2 eval)", tokens)
	}
	if cost != 0 {
		t.Errorf("cost = %v, want 0", cost)
	}
}

func TestOllamaParse_NoMatchDegradesToZero(t *testing.T) {
	b, _ := Get("ollama")
	if tokens, _ := b.Parse(nil, []byte("no stats here")); tokens != 0 {
		t.Errorf("tokens = %d, want 0", tokens)
	}
}
