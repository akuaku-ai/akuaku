// Package demo produces a deterministic, looping cast of agents for `akuaku
// demo` — a zero-setup way to see the monitor alive, and the source of the
// README's GIF. It only computes state; the command writes it to a scratch
// directory the monitor reads, so demo is just another producer on the same
// open-by-contract channel as everything else.
package demo

import (
	"time"

	"github.com/akuaku-ai/akuaku/internal/state"
)

// Period is the loop length in monitor ticks (~one second each). A recording of
// one period tiles seamlessly, since Frame is periodic.
const Period = 16

// costPerToken approximates a paid model's price so the demo's cost figure
// climbs believably. It is illustrative, not a real rate.
const costPerToken = 0.00035

// Frame is the deterministic state of the demo's agents at tick, as of base (the
// moment the demo started). It loops with Period. Every field is set except Dir,
// which the caller stamps so the agents fall inside the local scope.
func Frame(tick int, base time.Time) []state.Run {
	p := ((tick % Period) + Period) % Period

	// B: a codex run churning throughout — tokens climb, no cost reported.
	runs := []state.Run{
		agent("demo-b", "codex", "write tests for parser.go", "", state.StatusRunning, 120+p*75, base, 70*time.Second, nil),
	}

	// A: a claude refactor that finishes six seconds in and stays done.
	if p <= 5 {
		runs = append(runs, agent("demo-a", "claude", "refactor auth module", "opus-4.8", state.StatusRunning, 240+p*130, base, 200*time.Second, nil))
	} else {
		ended := base.Add(6 * time.Second)
		runs = append(runs, agent("demo-a", "claude", "refactor auth module", "opus-4.8", state.StatusDone, 890, base, 200*time.Second, &ended))
	}

	// C: a local ollama summary that pauses for input near the end of the loop.
	if p <= 11 {
		runs = append(runs, agent("demo-c", "ollama", "summarize the architecture doc", "llama3.1", state.StatusRunning, 80+p*60, base, 120*time.Second, nil))
	} else {
		runs = append(runs, agent("demo-c", "ollama", "summarize the architecture doc", "llama3.1", state.StatusWaiting, 740, base, 120*time.Second, nil))
	}

	// D: a claude review that waits for you, then resumes once you "answer".
	if p <= 8 {
		runs = append(runs, agent("demo-d", "claude", "review PR #42", "opus-4.8", state.StatusWaiting, 60, base, 45*time.Second, nil))
	} else {
		runs = append(runs, agent("demo-d", "claude", "review PR #42", "opus-4.8", state.StatusRunning, 60+(p-8)*110, base, 45*time.Second, nil))
	}

	// E: a new claude run that launches partway through — "and it launches, too".
	if p >= 9 {
		runs = append(runs, agent("demo-e", "claude", "add rate limiting", "opus-4.8", state.StatusRunning, 40+(p-9)*95, base, 5*time.Second, nil))
	}

	return runs
}

// agent builds one demo run: cost applies only to the paid backend, and the
// start time trails base by age so the run shows a realistic, non-zero duration.
func agent(id, backend, task, model string, status state.Status, tokens int, base time.Time, age time.Duration, ended *time.Time) state.Run {
	cost := 0.0
	if backend == "claude" {
		cost = float64(tokens) * costPerToken
	}
	return state.Run{
		ID:        id,
		Backend:   backend,
		Name:      task,
		Task:      task,
		Model:     model,
		Status:    status,
		Tokens:    tokens,
		Cost:      cost,
		StartedAt: base.Add(-age),
		EndedAt:   ended,
	}
}
