// Package backend defines the extensible set of agent CLIs Akuaku can launch.
// A backend knows how to build its non-interactive command and how to parse its
// output for token and cost usage. Adding a backend means adding a definition
// and registering it here — nothing in the launcher or monitor changes.
package backend

import (
	"fmt"
	"sort"
)

// Output is what a finished run yields: the agent's answer text and its usage.
// Every field is best-effort; unrecognized output leaves the zero value.
type Output struct {
	Text   string
	Tokens int
	Cost   float64
}

// Backend describes how to run and measure a specific agent CLI.
type Backend interface {
	// Key is the backend's unique identifier (e.g. "claude").
	Key() string
	// Command builds the executable name and arguments for a task, applying the
	// model when one is provided.
	Command(task, model string) (name string, args []string)
	// Parse extracts the answer text, token count, and cost from a finished
	// run's output. It is best-effort: unrecognized output yields a zero Output
	// without an error.
	Parse(stdout, stderr []byte) Output
}

// registry maps each backend key to its implementation. To add a backend,
// append it to the slice below.
var registry = func() map[string]Backend {
	backends := []Backend{claudeBackend{}, codexBackend{}, ollamaBackend{}}
	m := make(map[string]Backend, len(backends))
	for _, b := range backends {
		m[b.Key()] = b
	}
	return m
}()

// Get returns the backend registered under key, or an error if none exists.
func Get(key string) (Backend, error) {
	b, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("unknown backend %q (available: %v)", key, Keys())
	}
	return b, nil
}

// Keys returns the registered backend keys in sorted order.
func Keys() []string {
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
