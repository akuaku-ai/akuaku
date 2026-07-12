package backend

import (
	"regexp"
	"strconv"
	"strings"
)

// ollamaBackend drives a local model via `ollama run <model> ... --verbose`.
// The model is positional and has no server-side default, so callers must
// provide one.
type ollamaBackend struct{}

func (ollamaBackend) Key() string { return "ollama" }

func (ollamaBackend) Command(task, model string) (string, []string) {
	return "ollama", []string{"run", model, task, "--verbose"}
}

var (
	promptEvalRe = regexp.MustCompile(`prompt eval count:\s+(\d+)`)
	evalRe       = regexp.MustCompile(`(?m)^eval count:\s+(\d+)`)
)

// Parse reads Ollama's answer from stdout and its `--verbose` stats from stderr.
// Tokens are the sum of the prompt and generation eval counts; a local model has
// no cost.
func (ollamaBackend) Parse(stdout, stderr []byte) Output {
	return Output{
		Text:   strings.TrimSpace(string(stdout)),
		Tokens: firstInt(promptEvalRe, stderr) + firstInt(evalRe, stderr),
	}
}

// firstInt returns the first capture group of re parsed as an int, or zero.
func firstInt(re *regexp.Regexp, b []byte) int {
	match := re.FindSubmatch(b)
	if match == nil {
		return 0
	}
	n, _ := strconv.Atoi(string(match[1]))
	return n
}
