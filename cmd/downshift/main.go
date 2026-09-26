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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/adapters/claudecode"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/codex"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/cursor"
	"github.com/tiagovilasboas/harness-downshift/internal/benchmark"
	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/models"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/classifier"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/training"
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
			func(e claudecode.Event) string { return e.TaskText() },
		))
	case "cursor":
		os.Exit(runHookAdapter(
			os.Stdin,
			func(b []byte) (cursor.Event, error) { var e cursor.Event; return e, json.Unmarshal(b, &e) },
			func(e cursor.Event) (any, string, core.Decision) { return cursor.Handle(e, cat) },
			printCursorAllow,
			func(e cursor.Event) string { return e.TaskText() },
		))
	case "codex":
		os.Exit(runHookAdapter(
			os.Stdin,
			func(b []byte) (codex.Event, error) { var e codex.Event; return e, json.Unmarshal(b, &e) },
			func(e codex.Event) (any, string, core.Decision) { return codex.Handle(e, cat) },
			printCodexAllow,
			func(e codex.Event) string { return e.TaskText() },
		))
	case "try":
		os.Exit(runTry(cat, args[1:]))
	case "models":
		os.Exit(runModels(cat, args[1:]))
	case "stats":
		os.Exit(runStats(args[1:]))
	case "benchmark":
		os.Exit(runBenchmark(args[1:]))
	case "train":
		os.Exit(runTrain(args[1:]))
	case "feedback":
		os.Exit(runFeedback(args[1:]))
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
	taskText ...func(E) string,
) int {
	// Harness hook payloads contain task text. Bound the read so a malformed or
	// hostile stdin cannot make the persistent harness process exhaust memory.
	const maxHookPayloadBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(in, maxHookPayloadBytes+1))
	if err != nil {
		failOpen()
		return 0
	}
	if len(data) > maxHookPayloadBytes {
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
		if len(taskText) > 0 {
			if id, err := training.RecordRoutedDecision(taskText[0](ev), decision); err == nil && id != "" {
				fmt.Fprintf(os.Stderr, "downshift: feedback id %s (run `downshift feedback %s success|retry|failed` after review)\n", id, id)
			}
		}
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
// Usage: downshift stats [--days=N] [--cost-per-unit=USD]
//
// --days=N           time window in days (default 30; 0 = all time)
// --cost-per-unit=X  convert normalised units to dollars at rate X per unit
func runStats(args []string) int {
	opts := telemetry.StatsOptions{Days: 30}
	for _, arg := range args {
		switch {
		case len(arg) > 7 && arg[:7] == "--days=":
			n, err := strconv.ParseFloat(arg[7:], 64)
			if err != nil || n < 0 {
				fmt.Fprintf(os.Stderr, "invalid --days value: %s\n", arg[7:])
				return 2
			}
			opts.Days = int(n)
		case len(arg) > 16 && arg[:16] == "--cost-per-unit=":
			v, err := strconv.ParseFloat(arg[16:], 64)
			if err != nil || v < 0 {
				fmt.Fprintf(os.Stderr, "invalid --cost-per-unit value: %s\n", arg[16:])
				return 2
			}
			opts.CostPerUnit = v
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
			return 2
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
	telemetry.PrintStats(events, opts, os.Stdout)
	return 0
}

// runBenchmark runs the classifier against a labelled task dataset and prints
// a confusion matrix + false-downshift rate.
// Usage: downshift benchmark <dataset.json> [--compare]
func runBenchmark(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: downshift benchmark <dataset.json> [--compare]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "The dataset is a JSON array of {\"prompt\":\"...\",\"label\":\"TRIVIAL|SIMPLE|MEDIUM|COMPLEX\"} objects.")
		fmt.Fprintln(os.Stderr, "A seed dataset is available at benchmark/tasks.json in the repository.")
		fmt.Fprintln(os.Stderr, "Use --compare --candidate-weights=<file> to evaluate a candidate without activating it.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  --compare    Run both Legacy and CapabilityRouter v2 side by side")
		return 2
	}

	compare := false
	path := ""
	candidateWeights := ""
	for _, arg := range args {
		if arg == "--compare" {
			compare = true
		} else if strings.HasPrefix(arg, "--candidate-weights=") {
			candidateWeights = strings.TrimPrefix(arg, "--candidate-weights=")
		} else {
			path = arg
		}
	}

	if path == "" {
		fmt.Fprintln(os.Stderr, "error: dataset path required")
		return 2
	}

	if compare {
		return runBenchmarkCompare(path, candidateWeights)
	}

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

// runBenchmarkCompare runs Legacy vs CapabilityRouter v2 side by side.
// Uses the v2 training dataset format (SMALL/MID/FRONTIER labels).
func runBenchmarkCompare(path, candidateWeights string) int {
	// Load dataset in v2 format (SMALL/MID/FRONTIER)
	v2ds, err := training.LoadDataset(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading v2 dataset: %v\n", err)
		return 1
	}
	if v2ds.Size() == 0 {
		fmt.Fprintln(os.Stderr, "error: comparison dataset must contain at least one task")
		return 2
	}

	legacyTasks, err := legacyTasksForComparison(v2ds.Examples)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	// Legacy: map equivalent tiers to the old classifier's label vocabulary.
	legacyResults := benchmark.Run(legacyTasks, os.Stderr)

	// v2: run the capability router classifier
	clf := classifier.NewSoftmaxClassifierDefault()
	if candidateWeights != "" {
		weights, err := classifier.LoadWeightsFromFile(candidateWeights)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading candidate weights: %v\n", err)
			return 1
		}
		clf = classifier.NewSoftmaxClassifier(weights)
	}
	v2Actual := make([]core.Tier, v2ds.Size())
	v2Predicted := make([]core.Tier, v2ds.Size())

	for i, ex := range v2ds.Examples {
		v2Actual[i] = ex.Tier()
		probs, _ := clf.Classify(ex.Features)
		v2Predicted[i] = probs.MaxTier()
	}

	v2Metrics := training.Compute(v2Actual, v2Predicted)
	classCounts := map[core.Tier]int{}
	for _, tier := range v2Actual {
		classCounts[tier]++
	}

	// Compute legacy metrics in v2 terms (map COMPLEX→FRONTIER, etc.)
	legacyActual := make([]core.Tier, len(legacyResults))
	legacyPredicted := make([]core.Tier, len(legacyResults))
	for i, r := range legacyResults {
		// Convert legacy complexity to tier
		switch r.Task.Label {
		case "TRIVIAL":
			legacyActual[i] = core.TierSmall
		case "SIMPLE", "MEDIUM":
			legacyActual[i] = core.TierMid
		case "COMPLEX":
			legacyActual[i] = core.TierFrontier
		default:
			legacyActual[i] = core.TierMid
		}
		legacyPredicted[i] = r.Predicted.Tier()
	}
	legacyMetrics := training.Compute(legacyActual, legacyPredicted)

	// Print comparison
	fmt.Fprintln(os.Stdout, "Benchmark: Legacy vs Capability Router v2")
	if candidateWeights != "" {
		fmt.Fprintf(os.Stdout, "Candidate weights: %s\n", candidateWeights)
	}
	fmt.Fprintf(os.Stdout, "Dataset: %d tasks\n\n", v2ds.Size())
	fmt.Fprintf(os.Stdout, "Label support: SMALL=%d MID=%d FRONTIER=%d\n", classCounts[core.TierSmall], classCounts[core.TierMid], classCounts[core.TierFrontier])
	if classCounts[core.TierSmall] < 5 || classCounts[core.TierMid] < 5 || classCounts[core.TierFrontier] < 5 {
		fmt.Fprintln(os.Stdout, "Warning: fewer than 5 examples in at least one tier; treat this comparison as exploratory.")
	}

	modelLabel := "v2"
	if candidateWeights != "" {
		modelLabel = "Candidate"
	}
	fmt.Fprintf(os.Stdout, "%-35s %10s %10s %10s\n", "", "Legacy", modelLabel, "Delta")
	fmt.Fprintf(os.Stdout, "%-35s %10.1f%% %10.1f%% %+9.1f%%\n",
		"Tier accuracy",
		legacyMetrics.Accuracy*100, v2Metrics.Accuracy*100,
		(v2Metrics.Accuracy-legacyMetrics.Accuracy)*100)
	fmt.Fprintf(os.Stdout, "%-35s %10.1f%% %10.1f%% %+9.1f%%\n",
		"Unsafe downgrade",
		legacyMetrics.UnsafeDowngradeRate*100, v2Metrics.UnsafeDowngradeRate*100,
		(v2Metrics.UnsafeDowngradeRate-legacyMetrics.UnsafeDowngradeRate)*100)
	fmt.Fprintf(os.Stdout, "  %-33s %10d  %10d\n",
		"FRONTIER → SMALL",
		legacyMetrics.Unsafe.FrontierToSmall, v2Metrics.Unsafe.FrontierToSmall)
	fmt.Fprintf(os.Stdout, "  %-33s %10d  %10d\n",
		"FRONTIER → MID",
		legacyMetrics.Unsafe.FrontierToMid, v2Metrics.Unsafe.FrontierToMid)
	fmt.Fprintf(os.Stdout, "%-35s %10.1f%% %10.1f%% %+9.1f%%\n",
		"Over-routing",
		legacyMetrics.OverRoutingRate*100, v2Metrics.OverRoutingRate*100,
		(v2Metrics.OverRoutingRate-legacyMetrics.OverRoutingRate)*100)
	fmt.Fprintf(os.Stdout, "%-35s %10.3f  %10.3f  %+9.3f\n",
		"Risk-weighted loss",
		legacyMetrics.RiskWeightedLoss, v2Metrics.RiskWeightedLoss,
		v2Metrics.RiskWeightedLoss-legacyMetrics.RiskWeightedLoss)
	fmt.Fprintln(os.Stdout, "")

	// Verdict
	if v2Metrics.Unsafe.FrontierToSmall < legacyMetrics.Unsafe.FrontierToSmall ||
		v2Metrics.UnsafeDowngradeRate < legacyMetrics.UnsafeDowngradeRate {
		fmt.Fprintln(os.Stdout, "Recommendation: Capability v2 reduces unsafe downgrades.")
		fmt.Fprintln(os.Stdout, "                Accept any over-routing increase for safety gain.")
	} else if v2Metrics.Accuracy > legacyMetrics.Accuracy {
		fmt.Fprintln(os.Stdout, "Recommendation: Capability v2 improves overall accuracy.")
	} else {
		fmt.Fprintln(os.Stdout, "Recommendation: No clear improvement — keep legacy as default.")
		fmt.Fprintln(os.Stdout, "                Collect more training data and re-run train + compare.")
	}

	return 0
}

// runFeedback records engineer-reviewed results without coupling the loop to
// any harness completion protocol.
func runFeedback(args []string) int {
	if len(args) > 0 && args[0] == "stats" {
		if len(args) != 1 {
			fmt.Fprintln(os.Stderr, "usage: downshift feedback stats")
			return 2
		}
		store := training.NewEventStore(training.DefaultEventsPath())
		events, err := store.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading loop events: %v\n", err)
			return 1
		}
		type counts struct{ pending, success, retry, failed, trainable int }
		byHarness := make(map[string]counts)
		for _, event := range events {
			c := byHarness[event.Harness]
			if event.Outcome != nil && event.Outcome.RequiredTier != nil {
				c.trainable++
			}
			if event.Outcome == nil {
				c.pending++
			} else {
				switch {
				case event.Outcome.Success:
					c.success++
				case event.Outcome.Retry:
					c.retry++
				case event.Outcome.Failed:
					c.failed++
				}
			}
			byHarness[event.Harness] = c
		}
		if len(events) == 0 {
			fmt.Fprintln(os.Stdout, "No routing events yet.")
			return 0
		}
		fmt.Fprintf(os.Stdout, "%-16s %8s %8s %8s %8s %10s\n", "Harness", "Success", "Retry", "Failed", "Pending", "Trainable")
		var names []string
		for name := range byHarness {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			c := byHarness[name]
			fmt.Fprintf(os.Stdout, "%-16s %8d %8d %8d %8d %10d\n", name, c.success, c.retry, c.failed, c.pending, c.trainable)
		}
		return 0
	}
	if len(args) == 0 || args[0] == "list" {
		pendingOnly := len(args) == 2 && args[1] == "--pending"
		if len(args) > 2 || (len(args) == 2 && !pendingOnly) {
			fmt.Fprintln(os.Stderr, "usage: downshift feedback list [--pending] | stats")
			return 2
		}
		store := training.NewEventStore(training.DefaultEventsPath())
		events, err := store.Load()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading loop events: %v\n", err)
			return 1
		}
		if len(events) == 0 {
			fmt.Fprintln(os.Stdout, "No routing events yet. Run a subagent task through a configured harness first.")
			return 0
		}
		shown := 0
		for i := len(events) - 1; i >= 0 && shown < 50; i-- {
			event := events[i]
			if pendingOnly && event.Outcome != nil {
				continue
			}
			outcome := "pending"
			if event.Outcome != nil {
				switch {
				case event.Outcome.Success:
					outcome = "success"
				case event.Outcome.Retry:
					outcome = "retry → " + event.Outcome.RetryTier.String()
				case event.Outcome.Failed:
					outcome = "failed"
				}
			}
			fmt.Fprintf(os.Stdout, "%s  %-10s %-8s %-14s %s\n", event.ID, event.Harness, event.SelectedTier, outcome, event.Timestamp.Format(time.RFC3339))
			shown++
		}
		return 0
	}
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: downshift feedback list [--pending] | stats | <id> <success|retry|failed> [--retry-tier=MID|FRONTIER] [--required-tier=SMALL|MID|FRONTIER]")
		return 2
	}
	outcome := training.Outcome{}
	switch args[1] {
	case "success":
		outcome.Success = true
	case "retry":
		outcome.Retry = true
	case "failed":
		outcome.Failed = true
	default:
		fmt.Fprintf(os.Stderr, "unknown outcome %q; use success, retry, or failed\n", args[1])
		return 2
	}
	for _, arg := range args[2:] {
		if strings.HasPrefix(arg, "--retry-tier=") {
			switch strings.ToUpper(strings.TrimPrefix(arg, "--retry-tier=")) {
			case "MID":
				outcome.RetryTier = core.TierMid
			case "FRONTIER":
				outcome.RetryTier = core.TierFrontier
			default:
				fmt.Fprintln(os.Stderr, "retry tier must be MID or FRONTIER")
				return 2
			}
		} else if strings.HasPrefix(arg, "--required-tier=") {
			var required core.Tier
			switch strings.ToUpper(strings.TrimPrefix(arg, "--required-tier=")) {
			case "SMALL":
				required = core.TierSmall
			case "MID":
				required = core.TierMid
			case "FRONTIER":
				required = core.TierFrontier
			default:
				fmt.Fprintln(os.Stderr, "required tier must be SMALL, MID, or FRONTIER")
				return 2
			}
			outcome.RequiredTier = &required
		} else {
			fmt.Fprintf(os.Stderr, "unknown feedback option %q\n", arg)
			return 2
		}
	}
	store := training.NewEventStore(training.DefaultEventsPath())
	if err := store.AddOutcome(args[0], outcome); err != nil {
		fmt.Fprintf(os.Stderr, "error recording feedback: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Feedback recorded for %s. Train a candidate with `downshift train --from-events --output=candidate.json`.\n", args[0])
	return 0
}

func legacyTasksForComparison(examples []training.Example) ([]benchmark.Task, error) {
	labels := map[string]string{
		"SMALL":    "TRIVIAL",
		"MID":      "MEDIUM",
		"FRONTIER": "COMPLEX",
	}
	tasks := make([]benchmark.Task, 0, len(examples))
	for _, example := range examples {
		label, ok := labels[strings.ToUpper(strings.TrimSpace(example.Label))]
		if !ok {
			return nil, fmt.Errorf("unsupported tier %q; use SMALL, MID, or FRONTIER", example.Label)
		}
		tasks = append(tasks, benchmark.Task{Prompt: example.Prompt, Label: label})
	}
	return tasks, nil
}

// runTrain trains the capability router v2 classifier on a labelled dataset.
// Usage: downshift train <dataset.json> [--output=weights.json] [--validation-split=0.2] [--from-events]
func runTrain(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: downshift train <dataset.json> [--output=<path>] [--validation-split=0.2]")
		fmt.Fprintln(os.Stderr, "       downshift train --from-events [--output=<path>]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Dataset format: [{\"prompt\":\"...\",\"label\":\"SMALL|MID|FRONTIER\"}, ...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  --output=<path>          Where to write weights.json (default: ./weights.json)")
		fmt.Fprintln(os.Stderr, "  --validation-split=0.2   Fraction to use for validation (default: 0.2)")
		fmt.Fprintln(os.Stderr, "  --from-events            Train from collected routing events instead of a dataset")
		fmt.Fprintln(os.Stderr, "  --epochs=100             Number of training epochs (default: 100)")
		fmt.Fprintln(os.Stderr, "  --learning-rate=0.1      Learning rate for gradient descent (default: 0.1)")
		return 2
	}

	// Parse flags
	var datasetPath string
	outputPath := "weights.json"
	validationSplit := 0.2
	fromEvents := false
	config := training.DefaultTrainConfig()

	for _, arg := range args {
		switch {
		case arg == "--from-events":
			fromEvents = true
		case len(arg) > 9 && arg[:9] == "--output=":
			outputPath = arg[9:]
		case len(arg) > 19 && arg[:19] == "--validation-split=":
			v, err := strconv.ParseFloat(arg[19:], 64)
			if err == nil && v > 0 && v < 1 {
				validationSplit = v
			}
		case len(arg) > 9 && arg[:9] == "--epochs=":
			v, err := strconv.Atoi(arg[9:])
			if err == nil && v > 0 {
				config.Epochs = v
			}
		case len(arg) > 16 && arg[:16] == "--learning-rate=":
			v, err := strconv.ParseFloat(arg[16:], 64)
			if err == nil && v > 0 {
				config.LearningRate = v
			}
		default:
			if !fromEvents {
				datasetPath = arg
			}
		}
	}

	fmt.Fprintln(os.Stdout, "Training capability-router v2")
	fmt.Fprintln(os.Stdout, "")

	var result training.TrainResult
	var err error

	if fromEvents {
		eventsPath := training.DefaultEventsPath()
		fmt.Fprintf(os.Stdout, "Source:  events file (%s)\n", eventsPath)
		store := training.NewEventStore(eventsPath)
		if _, err = store.Load(); err == nil {
			labeled := store.ToDataset()
			fmt.Fprintf(os.Stdout, "Reviewed minimum-tier labels: %d\n", labeled.Size())
			trainDS, valDS := labeled.Split(validationSplit)
			fmt.Fprintf(os.Stdout, "Train/Val: %d / %d\n\n", trainDS.Size(), valDS.Size())
			result = training.Train(trainDS, valDS, config)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading events: %v\n", err)
			return 1
		}
		if result.Weights == nil {
			fmt.Fprintln(os.Stderr, "no reviewed minimum-tier labels yet — add --required-tier=SMALL|MID|FRONTIER when recording feedback")
			return 1
		}
	} else {
		if datasetPath == "" {
			fmt.Fprintln(os.Stderr, "error: dataset path required (or use --from-events)")
			return 2
		}

		ds, loadErr := training.LoadDataset(datasetPath)
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "error loading dataset: %v\n", loadErr)
			return 1
		}

		fmt.Fprintf(os.Stdout, "Dataset: %d tasks\n", ds.Size())
		trainDS, valDS := ds.Split(validationSplit)
		fmt.Fprintf(os.Stdout, "Train/Val: %d / %d\n\n", trainDS.Size(), valDS.Size())

		result = training.Train(trainDS, valDS, config)
	}

	// Print training results
	fmt.Fprintf(os.Stdout, "Epochs: %d  LR: %.3f  Risk-weighted: %v\n\n",
		config.Epochs, config.LearningRate, config.RiskWeighted)
	fmt.Fprintf(os.Stdout, "Training metrics:\n%s\n", result.TrainMetrics.Summary())
	if result.ValMetrics.ConfusionMatrix.Total() > 0 {
		fmt.Fprintf(os.Stdout, "Validation metrics:\n%s\n", result.ValMetrics.Summary())
	}

	// Save weights
	if err := classifier.SaveWeights(result.Weights, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "error saving weights: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stdout, "Weights saved to: %s\n", outputPath)
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintf(os.Stdout, "To evaluate: downshift benchmark <holdout.json> --compare --candidate-weights=%s\n", outputPath)
	fmt.Fprintln(os.Stdout, "Candidate weights are not activated automatically; review safety and quality metrics before promotion.")
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
  downshift stats --cost-per-unit=<USD>   Convert normalised units to dollars
  downshift benchmark <file>     Run classifier against a labelled dataset; print confusion matrix
  downshift benchmark <file> --compare  Compare Legacy vs CapabilityRouter v2 side by side
  downshift train <file>         Train capability-router v2 on a labelled dataset
  downshift train --from-events  Train from engineer-reviewed local feedback
  downshift feedback list        List routing IDs awaiting engineer review
  downshift feedback stats       Summarize outcomes per harness
  downshift feedback <id> <outcome>  Record success, retry, or failed

Grok note:
  Grok routes subagent models via config, not a hook (its PreToolUse is
  allow/deny only). Run  downshift try "<task>" grok  to see the reasoning
  effort to pin on a subagent role in ~/.grok/config.toml. See the README.

Examples:
  downshift try "rename the variable userId"
  downshift try "rearchitect the payment flow" cursor claude-haiku-4-5
  downshift try "add a subagent to scan for secrets" codex gpt-5.6-sol
  downshift try "explore the auth module" grok
  downshift stats
  downshift stats --days=7
  downshift benchmark benchmark/tasks.json
`)
}
