// Command downshift is the harness-downshift binary. It runs as a hook: a
// harness pipes a JSON event on stdin, downshift classifies the subagent task
// and prints the steering JSON on stdout that rewrites the subagent's model.
//
// Usage as a Claude Code PreToolUse hook:
//
//	{ "type": "command", "command": "downshift claude-code" }
//
// It also has a `try` subcommand to test classification from the terminal:
//
//	downshift try "rename the variable userId"
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tiagovilasboas/harness-downshift/internal/adapters/claudecode"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/codex"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/cursor"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	switch args[0] {
	case "claude-code":
		os.Exit(runHookAdapter(
			func(b []byte) (claudecode.Event, error) { var e claudecode.Event; return e, json.Unmarshal(b, &e) },
			func(e claudecode.Event) (any, string) { return claudecode.Handle(e) },
			printAllow,
		))
	case "cursor":
		os.Exit(runHookAdapter(
			func(b []byte) (cursor.Event, error) { var e cursor.Event; return e, json.Unmarshal(b, &e) },
			func(e cursor.Event) (any, string) { return cursor.Handle(e) },
			printCursorAllow,
		))
	case "codex":
		os.Exit(runHookAdapter(
			func(b []byte) (codex.Event, error) { var e codex.Event; return e, json.Unmarshal(b, &e) },
			func(e codex.Event) (any, string) { return codex.Handle(e) },
			printCodexAllow,
		))
	case "try":
		os.Exit(runTry(args[1:]))
	case "-h", "--help", "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		usage()
		os.Exit(2)
	}
}

// runHookAdapter is the single hook-runner template shared by all harness
// adapters. It reads a JSON event from stdin, calls handle, and encodes the
// result to stdout. Any failure (read error, parse error, encode error) prints
// the harness-specific fail-open response and exits 0 — the spawn must never
// be blocked by a router error.
//
// Type parameter E is the harness-specific event struct.
func runHookAdapter[E any](
	parse func([]byte) (E, error),
	handle func(E) (any, string),
	failOpen func(),
) int {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		failOpen()
		return 0
	}
	ev, err := parse(data)
	if err != nil {
		failOpen()
		return 0
	}
	out, note := handle(ev)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		failOpen()
		return 0
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, "downshift: "+note)
	}
	return 0
}

// runTry classifies a prompt from the command line for quick testing.
func runTry(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: downshift try \"<task prompt>\" [harness] [current-model]")
		return 2
	}
	prompt := args[0]
	harness := "claude-code"
	current := ""
	if len(args) >= 2 {
		harness = args[1]
	}
	if len(args) >= 3 {
		current = args[2]
	}

	// Grok uses a single model with configurable reasoning — report effort
	// instead of a model ID, and point at the config-based mechanism.
	if harness == "grok" {
		cls := core.Classify(prompt)
		tier := cls.Complexity.Tier()
		effort := core.EffortFor(tier) // Wave 4 promotes this; for now mirrors grokEffortFor
		fmt.Printf("Task:       %s\n", prompt)
		fmt.Printf("Complexity: %s\n", cls.Complexity)
		fmt.Printf("Needs tier: %s\n", tier)
		fmt.Printf("Reasoning:  %s  (grok-4.6, configurable reasoning)\n", effort)
		fmt.Printf("→ set reasoning_effort=%q on the subagent role/persona in config.toml\n", effort.String())
		return 0
	}

	d := core.Route(prompt, harness, current)
	fmt.Printf("Task:       %s\n", prompt)
	fmt.Printf("Complexity: %s\n", d.Complexity)
	fmt.Printf("Needs tier: %s\n", d.Tier)
	fmt.Printf("Recommend:  %s\n", d.Model.ID)
	if d.CurrentModel.ID != "" {
		fmt.Printf("Current:    %s\n", d.CurrentModel.ID)
	}
	fmt.Printf("Verdict:    %s\n", d.Verdict)
	fmt.Printf("→ %s\n", d.Summary())
	return 0
}

// --- Harness-specific fail-open responses ---
// Each harness has a different envelope for "allow unchanged". These are the
// minimal valid JSON outputs that let the tool call proceed unmodified.

func printAllow() {
	fmt.Println(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`)
}

func printCursorAllow() {
	fmt.Println(`{"permission":"allow"}`)
}

func printCodexAllow() {
	fmt.Println(`{"continue":true,"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`)
}

func usage() {
	fmt.Fprint(os.Stderr, `downshift — right-sized models for every subagent task

Usage:
  downshift claude-code          Run as a Claude Code PreToolUse hook (reads stdin)
  downshift cursor               Run as a Cursor preToolUse hook (reads stdin)
  downshift codex                Run as a Codex PreToolUse hook (reads stdin)
  downshift try "<task>" [harness] [model]   Test classification from the terminal

Grok note:
  Grok routes subagent models via config, not a hook (its PreToolUse is
  allow/deny only). Run  downshift try "<task>" grok  to see the reasoning
  effort to pin on a subagent role in ~/.grok/config.toml. See the README.

Examples:
  downshift try "rename the variable userId"
  downshift try "rearchitect the payment flow" cursor claude-haiku-4
  downshift try "add a subagent to scan for secrets" codex gpt-5.3-codex
  downshift try "explore the auth module" grok
`)
}
