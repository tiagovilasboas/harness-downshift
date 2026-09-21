// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

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
	"strconv"

	"github.com/tiagovilasboas/harness-downshift/internal/adapters/claudecode"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/codex"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/cursor"
	"github.com/tiagovilasboas/harness-downshift/internal/benchmark"
	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/models"
	"github.com/tiagovilasboas/harness-downshift/internal/telemetry"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	// Load the effective catalog once at startup — embedded JSON with optional
	// user override at ~/.harness-downshift/catalog.json. Fail-open: if the
	// user file is invalid, the embedded catalog is used automatically.
	cat := catalog.Load()

	switch args[0] {
	case "claude-code":
		os.Exit(runHookAdapter(
			os.Stdin,
			func(b []byte) (claudecode.Event, error) { var e claudecode.Event; return e, json.Unmarshal(b, &e) },
			func(e claudecode.Event) (any, string, core.Decision) { return claudecode.Handle(e, cat) },
			printAllow,
		))
	case "cursor":
		os.Exit(runHookAdapter(
			os.Stdin,
			func(b []byte) (cursor.Event, error) { var e cursor.Event; return e, json.Unmarshal(b, &e) },
			func(e cursor.Event) (any, string, core.Decision) { return cursor.Handle(e, cat) },
			printCursorAllow,
		))
	case "codex":
		os.Exit(runHookAdapter(
			os.Stdin,
			func(b []byte) (codex.Event, error) { var e codex.Event; return e, json.Unmarshal(b, &e) },
			func(e codex.Event) (any, string, core.Decision) { return codex.Handle(e, cat) },
			printCodexAllow,
		))
	case "try":
		os.Exit(runTry(cat, args[1:]))
	case "models":
		os.Exit(runModels(cat, args[1:]))
	case "stats":
		os.Exit(runStats(args[1:]))
	case "benchmark":
		os.Exit(runBenchmark(args[1:]))
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
// adapters. It reads a JSON event from in, calls handle, encodes the result
// to stdout, and records telemetry for any decision with a non-empty harness.
// Any failure prints the harness-specific fail-open response and exits 0 —
// the spawn must never be blocked by a router error.
func runHookAdapter[E any](
	in io.Reader,
	parse func([]byte) (E, error),
	handle func(E) (any, string, core.Decision),
	failOpen func(),
) int {
	data, err := io.ReadAll(in)
	if err != nil {
		failOpen()
		return 0
	}
	ev, err := parse(data)
	if err != nil {
		failOpen()
		return 0
	}
	out, note, decision := handle(ev)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		failOpen()
		return 0
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, "downshift: "+note)
	}
	// Record telemetry only when a real routing decision was made.
	// A zero Decision (Harness == "") means the event was not a subagent spawn.
	if decision.Harness != "" {
		telemetry.Record(telemetry.FromDecision(decision))
	}
	return 0
}

// runTry classifies a prompt from the command line for quick testing.
func runTry(cat core.Resolver, args []string) int {
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
		effort := core.EffortFor(tier)
		fmt.Printf("Task:       %s\n", prompt)
		fmt.Printf("Complexity: %s\n", cls.Complexity)
		fmt.Printf("Needs tier: %s\n", tier)
		fmt.Printf("Reasoning:  %s  (grok-4.6, configurable reasoning)\n", effort)
		fmt.Printf("→ set reasoning_effort=%q on the subagent role/persona in config.toml\n", effort.String())
		return 0
	}

	d := core.Route(prompt, harness, current, cat)
	fmt.Printf("Task:       %s\n", prompt)
	fmt.Printf("Complexity: %s\n", d.Complexity)
	fmt.Printf("Intent:     %s\n", d.Intent)
	fmt.Printf("Needs tier: %s\n", d.Tier)
	fmt.Printf("Recommend:  %s\n", d.Model.ID)
	if d.CurrentModel.ID != "" {
		fmt.Printf("Current:    %s\n", d.CurrentModel.ID)
	}
	fmt.Printf("Verdict:    %s\n", d.Verdict)
	fmt.Printf("→ %s\n", d.Summary())
	return 0
}

// runModels dispatches the 'models' subcommands: list, check, pull.
func runModels(cat models.CatalogReader, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: downshift models <list|check|pull>")
		return 2
	}
	switch args[0] {
	case "list":
		models.List(cat, os.Stdout)
		return 0
	case "check":
		return models.Check(cat, os.Stdout, os.Stderr)
	case "pull":
		return models.Pull(cat, os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "unknown models subcommand %q\n", args[0])
		return 2
	}
}

// runStats reads the local event log and prints a savings summary.
// Usage: downshift stats [--days=N]  (default: last 30 days; 0 = all time)
func runStats(args []string) int {
	days := 30
	for _, arg := range args {
		if len(arg) > 7 && arg[:7] == "--days=" {
			n, err := strconv.Atoi(arg[7:])
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid --days value: %s\n", arg[7:])
				return 2
			}
			days = n
		}
	}

	events, err := telemetry.ReadEvents()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading event log: %v\n", err)
		return 1
	}
	if len(events) == 0 {
		fmt.Fprintln(os.Stderr, "no events recorded yet — run some subagent tasks first.")
		return 0
	}
	telemetry.PrintStats(events, days, os.Stdout)
	return 0
}

// runBenchmark runs the classifier against a labelled task dataset and prints
// a confusion matrix + false-downshift rate.
// Usage: downshift benchmark <dataset.json>
func runBenchmark(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: downshift benchmark <dataset.json>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "The dataset is a JSON array of {\"prompt\":\"...\",\"label\":\"TRIVIAL|SIMPLE|MEDIUM|COMPLEX\"} objects.")
		fmt.Fprintln(os.Stderr, "A seed dataset is available at benchmark/tasks.json in the repository.")
		return 2
	}
	path := args[0]
	tasks, err := benchmark.LoadDataset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading dataset: %v\n", err)
		return 1
	}
	results := benchmark.Run(tasks, os.Stderr)
	matrix := benchmark.Build(results)
	benchmark.Print(results, matrix, os.Stdout)
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
	fmt.Println(`{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}`)
}

func usage() {
	fmt.Fprint(os.Stderr, `downshift — right-sized models for every subagent task

Usage:
  downshift claude-code          Run as a Claude Code PreToolUse hook (reads stdin)
  downshift cursor               Run as a Cursor preToolUse hook (reads stdin)
  downshift codex                Run as a Codex PreToolUse hook (reads stdin)
  downshift try "<task>" [harness] [model]   Test classification from the terminal
  downshift models list          Show the effective catalog (embedded or override)
  downshift models check         Query provider APIs and report new/untiered models
  downshift models pull          Write ~/.harness-downshift/catalog.json from APIs
  downshift stats [--days=N]     Show routing decisions and estimated savings (default: 30 days)
  downshift benchmark <file>     Run classifier against a labelled dataset; print confusion matrix

Grok note:
  Grok routes subagent models via config, not a hook (its PreToolUse is
  allow/deny only). Run  downshift try "<task>" grok  to see the reasoning
  effort to pin on a subagent role in ~/.grok/config.toml. See the README.

Examples:
  downshift try "rename the variable userId"
  downshift try "rearchitect the payment flow" cursor claude-haiku-4
  downshift try "add a subagent to scan for secrets" codex gpt-5.6-sol
  downshift try "explore the auth module" grok
  downshift stats
  downshift stats --days=7
  downshift benchmark benchmark/tasks.json
`)
}
