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
		os.Exit(runClaudeCode())
	case "cursor":
		os.Exit(runCursor())
	case "codex":
		os.Exit(runCodex())
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

// runClaudeCode reads a PreToolUse event on stdin and prints the steering JSON.
// It always exits 0 and always prints a valid "allow" decision, so a parse
// failure never blocks the user's tool call (fail-open).
func runClaudeCode() int {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		printAllow()
		return 0
	}

	var ev claudecode.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		printAllow()
		return 0
	}

	out, note := claudecode.Handle(ev)
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		printAllow()
		return 0
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, "downshift: "+note)
	}
	return 0
}

// runCursor reads a preToolUse event on stdin and prints Cursor's steering
// JSON. Fail-open: any failure prints a plain allow and exits 0.
func runCursor() int {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		printCursorAllow()
		return 0
	}

	var ev cursor.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		printCursorAllow()
		return 0
	}

	out, note := cursor.Handle(ev)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		printCursorAllow()
		return 0
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, "downshift: "+note)
	}
	return 0
}

// runCodex reads a PreToolUse event on stdin and prints Codex's steering JSON
// for multi_agent_v2 spawn_agent calls. Fail-open: any failure prints a plain
// allow (continue:true) and exits 0.
func runCodex() int {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		printCodexAllow()
		return 0
	}

	var ev codex.Event
	if err := json.Unmarshal(data, &ev); err != nil {
		printCodexAllow()
		return 0
	}

	out, note := codex.Handle(ev)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		printCodexAllow()
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

	// Grok ships a single coding model with configurable reasoning, so the
	// lever there is reasoning effort, not model tier. Report that instead of a
	// misleading model id, and point at the config-based mechanism.
	if harness == "grok" {
		cls := core.Classify(prompt)
		tier := cls.Complexity.Tier()
		fmt.Printf("Task:       %s\n", prompt)
		fmt.Printf("Complexity: %s\n", cls.Complexity)
		fmt.Printf("Needs tier: %s\n", tier)
		fmt.Printf("Reasoning:  %s  (grok-4.6, configurable reasoning)\n", grokEffortFor(tier))
		fmt.Printf("→ set reasoning_effort=%q on the subagent role/persona in config.toml\n", grokEffortFor(tier))
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

// grokEffortFor maps a complexity tier to a Grok reasoning_effort level. Grok
// accepts low, medium, and high. Cheaper tiers get lower effort.
func grokEffortFor(tier core.Tier) string {
	switch tier {
	case core.TierSmall:
		return "low"
	case core.TierMid:
		return "medium"
	default:
		return "high"
	}
}

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
