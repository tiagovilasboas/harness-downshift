// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package-level tests for cmd/downshift. These call the internal functions
// directly so go test -cover can instrument them, unlike the subprocess-based
// main_test.go which builds and runs a separate binary.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/adapters/claudecode"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/codex"
	"github.com/tiagovilasboas/harness-downshift/internal/adapters/cursor"
	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/classifier"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/training"
)

// captureStdout redirects os.Stdout to a buffer for the duration of fn.
func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

var cmdCat = catalog.Load()

func TestLegacyTasksForComparisonMapsV2Tiers(t *testing.T) {
	tasks, err := legacyTasksForComparison([]training.Example{
		{Prompt: "rename a variable", Label: "small"},
		{Prompt: "add a field", Label: "MID"},
		{Prompt: "rearchitect payments", Label: "FRONTIER"},
	})
	if err != nil {
		t.Fatalf("legacyTasksForComparison() error = %v", err)
	}
	want := []string{"TRIVIAL", "MEDIUM", "COMPLEX"}
	if len(tasks) != len(want) {
		t.Fatalf("got %d tasks, want %d", len(tasks), len(want))
	}
	for i := range want {
		if tasks[i].Label != want[i] {
			t.Errorf("task %d label = %q, want %q", i, tasks[i].Label, want[i])
		}
	}
}

func TestLegacyTasksForComparisonRejectsUnknownTier(t *testing.T) {
	_, err := legacyTasksForComparison([]training.Example{{Prompt: "task", Label: "UNKNOWN"}})
	if err == nil {
		t.Fatal("legacyTasksForComparison() expected an error for an unsupported tier")
	}
}

// claudeCodeFrontierID returns the current frontier model ID for claude-code.
func claudeCodeFrontierID() string { return cmdCat.ModelFor("claude-code", core.TierFrontier).ID }

// --- runTry ---

func TestRunTry_NoArgs(t *testing.T) {
	if rc := runTry(cmdCat, nil); rc != 2 {
		t.Errorf("no args rc = %d, want 2", rc)
	}
}

func TestRunTry_TrivialTask(t *testing.T) {
	out := captureStdout(func() {
		if rc := runTry(cmdCat, []string{"rename the userId variable", "claude-code"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	for _, want := range []string{"TRIVIAL", "trivial", "small"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output; got:\n%s", want, out)
		}
	}
}

func TestRunTry_ComplexTask(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"rearchitect the payment flow across services", "claude-code"})
	})
	if !strings.Contains(out, "COMPLEX") {
		t.Errorf("expected COMPLEX; got:\n%s", out)
	}
	if !strings.Contains(out, "normal") {
		t.Errorf("expected 'normal' intent (construction, not review); got:\n%s", out)
	}
}

func TestRunTry_CodeReviewIntent(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"do a code review of the auth module", "claude-code"})
	})
	if !strings.Contains(out, "review") {
		t.Errorf("expected 'review' intent; got:\n%s", out)
	}
}

func TestRunTry_WithCurrentModel_ShowsVerdict(t *testing.T) {
	frontierID := claudeCodeFrontierID()
	out := captureStdout(func() {
		runTry(cmdCat, []string{"rename the userId variable", "claude-code", frontierID})
	})
	if !strings.Contains(out, "DOWNSHIFT") {
		t.Errorf("expected DOWNSHIFT verdict; got:\n%s", out)
	}
	if !strings.Contains(out, "Current:") {
		t.Errorf("expected Current: line in output; got:\n%s", out)
	}
}

func TestRunTry_Grok_TrivialEffort(t *testing.T) {
	out := captureStdout(func() {
		if rc := runTry(cmdCat, []string{"rename the userId variable", "grok"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(out, "grok-4.6") {
		t.Errorf("expected grok-4.6 model; got:\n%s", out)
	}
	if !strings.Contains(out, "low") {
		t.Errorf("expected 'low' effort for trivial grok task; got:\n%s", out)
	}
}

func TestRunTry_Grok_ComplexEffort(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"rearchitect the auth system across services", "grok"})
	})
	if !strings.Contains(out, "high") {
		t.Errorf("expected 'high' effort for complex grok task; got:\n%s", out)
	}
}

func TestRunTry_Grok_ReviewEffort(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"do a security audit of the payment service", "grok"})
	})
	if !strings.Contains(out, "high") {
		t.Errorf("expected 'high' effort for review grok task; got:\n%s", out)
	}
}

// --- runModels ---

func TestRunModels_NoArgs(t *testing.T) {
	if rc := runModels(cmdCat, nil); rc != 2 {
		t.Errorf("no args rc = %d, want 2", rc)
	}
}

func TestRunModels_UnknownSubcommand(t *testing.T) {
	if rc := runModels(cmdCat, []string{"unknown-subcmd"}); rc != 2 {
		t.Errorf("unknown subcommand rc = %d, want 2", rc)
	}
}

func TestRunModels_List(t *testing.T) {
	out := captureStdout(func() {
		if rc := runModels(cmdCat, []string{"list"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	for _, want := range []string{"claude-code", "codex", "Catalog source:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in list output; got:\n%s", want, out)
		}
	}
}

func TestRunModels_Check_NoKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	out := captureStdout(func() {
		if rc := runModels(cmdCat, []string{"check"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(out, "No providers checked") {
		t.Errorf("expected 'No providers checked'; got:\n%s", out)
	}
}

func TestRunModels_Pull_NoKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	captureStdout(func() {
		if rc := runModels(cmdCat, []string{"pull"}); rc != 0 {
			t.Errorf("pull with no keys rc = %d, want 0", rc)
		}
	})
}

// --- runHookAdapter ---

// hookEvent builds a minimal PreToolUse JSON event for the given harness.
func hookEvent(toolName, modelID, prompt string) []byte {
	return []byte(`{"hook_event_name":"PreToolUse","tool_name":"` + toolName +
		`","model":"` + modelID + `","tool_input":{"prompt":"` + prompt +
		`","model":"` + modelID + `"}}`)
}

func TestRunHookAdapter_ClaudeCode_Downshift(t *testing.T) {
	frontierID := claudeCodeFrontierID()
	event := hookEvent("Task", frontierID, "rename the userId variable")

	out := captureStdout(func() {
		rc := runHookAdapter(
			bytes.NewReader(event),
			func(b []byte) (claudecode.Event, error) { var e claudecode.Event; return e, json.Unmarshal(b, &e) },
			func(e claudecode.Event) (any, string, core.Decision) { return claudecode.Handle(e, cmdCat) },
			printAllow,
		)
		if rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Errorf("expected allow decision; got:\n%s", out)
	}
	if !strings.Contains(out, `"updatedInput"`) {
		t.Errorf("expected updatedInput (downshift); got:\n%s", out)
	}
}

func TestRunHookAdapter_Cursor_Downshift(t *testing.T) {
	frontierID := claudeCodeFrontierID() // cursor also uses claude-code catalog
	event := []byte(`{"hook_event_name":"preToolUse","tool_name":"Task","model_id":"` +
		frontierID + `","tool_input":{"task":"rename the userId variable","model":"` + frontierID + `"}}`)

	out := captureStdout(func() {
		runHookAdapter(
			bytes.NewReader(event),
			func(b []byte) (cursor.Event, error) { var e cursor.Event; return e, json.Unmarshal(b, &e) },
			func(e cursor.Event) (any, string, core.Decision) { return cursor.Handle(e, cmdCat) },
			printCursorAllow,
		)
	})
	if !strings.Contains(out, `"permission":"allow"`) {
		t.Errorf("expected cursor allow; got:\n%s", out)
	}
}

func TestRunHookAdapter_Codex_Downshift(t *testing.T) {
	frontierID := cmdCat.ModelFor("codex", core.TierFrontier).ID
	event := []byte(`{"hook_event_name":"PreToolUse","tool_name":"spawn_agent","model":"` +
		frontierID + `","tool_input":{"message":"rename the userId variable","model":"` + frontierID + `"}}`)

	out := captureStdout(func() {
		runHookAdapter(
			bytes.NewReader(event),
			func(b []byte) (codex.Event, error) { var e codex.Event; return e, json.Unmarshal(b, &e) },
			func(e codex.Event) (any, string, core.Decision) { return codex.Handle(e, cmdCat) },
			printCodexAllow,
		)
	})
	if !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Errorf("expected codex allow decision; got:\n%s", out)
	}
	if !strings.Contains(out, `"updatedInput"`) {
		t.Errorf("expected updatedInput (downshift); got:\n%s", out)
	}
	if !strings.Contains(out, `"reasoning_effort"`) {
		t.Errorf("expected reasoning_effort in codex output; got:\n%s", out)
	}
}

func TestHookDecisionRecordsFeedbackWithoutPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	frontierID := cmdCat.ModelFor("codex", core.TierFrontier).ID
	privatePrompt := "review private incident token abc123"
	event := []byte(`{"hook_event_name":"PreToolUse","tool_name":"spawn_agent","model":"` + frontierID + `","tool_input":{"message":"` + privatePrompt + `"}}`)
	captureStdout(func() {
		runHookAdapter(
			bytes.NewReader(event),
			func(b []byte) (codex.Event, error) { var e codex.Event; return e, json.Unmarshal(b, &e) },
			func(e codex.Event) (any, string, core.Decision) { return codex.Handle(e, cmdCat) },
			printCodexAllow,
			func(e codex.Event) string { return e.TaskText() },
		)
	})
	data, err := os.ReadFile(training.DefaultEventsPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), privatePrompt) || strings.Contains(string(data), "abc123") {
		t.Fatal("hook feedback log persisted raw prompt content")
	}
	events, err := training.NewEventStore(training.DefaultEventsPath()).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID == "" || events[0].Harness != "codex" {
		t.Fatalf("expected a routed codex event with feedback ID, got %#v", events)
	}
}

func TestRunHookAdapter_MalformedJSON_FailOpen(t *testing.T) {
	out := captureStdout(func() {
		rc := runHookAdapter(
			bytes.NewReader([]byte(`{not valid json`)),
			func(b []byte) (claudecode.Event, error) { var e claudecode.Event; return e, json.Unmarshal(b, &e) },
			func(e claudecode.Event) (any, string, core.Decision) { return claudecode.Handle(e, cmdCat) },
			printAllow,
		)
		if rc != 0 {
			t.Errorf("malformed JSON must fail-open (rc=0), got %d", rc)
		}
	})
	if !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Errorf("fail-open must print allow; got:\n%s", out)
	}
}

func TestRunHookAdapter_OversizedPayload_FailOpen(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), (1<<20)+1)
	out := captureStdout(func() {
		rc := runHookAdapter(
			bytes.NewReader(payload),
			func(b []byte) (claudecode.Event, error) { var e claudecode.Event; return e, json.Unmarshal(b, &e) },
			func(e claudecode.Event) (any, string, core.Decision) { return claudecode.Handle(e, cmdCat) },
			printAllow,
		)
		if rc != 0 {
			t.Errorf("oversized payload must fail-open (rc=0), got %d", rc)
		}
	})
	if !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Errorf("oversized payload must print allow; got:\n%s", out)
	}
}

func TestRunFeedbackRecordsHarnessAgnosticOutcome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := training.NewEventStore(training.DefaultEventsPath())
	if err := store.Record(training.Event{ID: "route-test", Harness: "cursor", SelectedTier: core.TierSmall}); err != nil {
		t.Fatal(err)
	}
	if rc := runFeedback([]string{"route-test", "success", "--required-tier=SMALL"}); rc != 0 {
		t.Fatalf("runFeedback() = %d, want 0", rc)
	}
	events, err := training.NewEventStore(training.DefaultEventsPath()).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Outcome == nil || !events[0].Outcome.Success || events[0].Outcome.RequiredTier == nil || *events[0].Outcome.RequiredTier != core.TierSmall {
		t.Fatalf("feedback event = %#v", events)
	}
	stats := captureStdout(func() {
		if rc := runFeedback([]string{"stats"}); rc != 0 {
			t.Fatalf("feedback stats rc = %d", rc)
		}
	})
	if !strings.Contains(stats, "cursor") || !strings.Contains(stats, "Success") {
		t.Fatalf("feedback stats missing harness outcome: %s", stats)
	}
}

func TestRunFeedbackRejectsInvalidRetry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := training.NewEventStore(training.DefaultEventsPath())
	if err := store.Record(training.Event{ID: "route-mid", Harness: "codex", SelectedTier: core.TierMid}); err != nil {
		t.Fatal(err)
	}
	if rc := runFeedback([]string{"route-mid", "retry", "--retry-tier=MID"}); rc == 0 {
		t.Fatal("retry at the same tier should be rejected")
	}
}

func TestFeedbackEventStoreUsesSeparateLoopLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if filepath.Base(training.DefaultEventsPath()) != "loop-events.jsonl" {
		t.Fatalf("loop event file is not isolated from routing stats: %s", training.DefaultEventsPath())
	}
}

func TestEngineeringLoopEndToEnd(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	frontierID := cmdCat.ModelFor("codex", core.TierFrontier).ID
	event := []byte(`{"hook_event_name":"PreToolUse","tool_name":"spawn_agent","model":"` + frontierID + `","tool_input":{"message":"rename the userId variable"}}`)
	captureStdout(func() {
		if rc := runHookAdapter(
			bytes.NewReader(event),
			func(b []byte) (codex.Event, error) { var e codex.Event; return e, json.Unmarshal(b, &e) },
			func(e codex.Event) (any, string, core.Decision) { return codex.Handle(e, cmdCat) },
			printCodexAllow,
			func(e codex.Event) string { return e.TaskText() },
		); rc != 0 {
			t.Fatalf("hook rc = %d", rc)
		}
	})
	events, err := training.NewEventStore(training.DefaultEventsPath()).Load()
	if err != nil || len(events) != 1 {
		t.Fatalf("events = %d, err = %v", len(events), err)
	}
	if rc := runFeedback([]string{events[0].ID, "success", "--required-tier=SMALL"}); rc != 0 {
		t.Fatalf("feedback rc = %d", rc)
	}

	candidate := filepath.Join(t.TempDir(), "candidate.json")
	result, err := training.TrainFromEvents(training.DefaultEventsPath(), training.DefaultTrainConfig())
	if err != nil || result.Weights == nil {
		t.Fatalf("TrainFromEvents result=%+v err=%v", result, err)
	}
	if err := classifier.SaveWeights(result.Weights, candidate); err != nil {
		t.Fatal(err)
	}
	dataset := filepath.Join(t.TempDir(), "holdout.json")
	contents := `[{"prompt":"rename a local variable","label":"SMALL"},{"prompt":"implement a CSV export feature","label":"MID"},{"prompt":"rearchitect authentication across services","label":"FRONTIER"}]`
	if err := os.WriteFile(dataset, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureStdout(func() {
		if rc := runBenchmark([]string{dataset, "--compare", "--candidate-weights=" + candidate}); rc != 0 {
			t.Fatalf("benchmark rc = %d", rc)
		}
	})
	if !strings.Contains(output, "Candidate weights:") {
		t.Fatalf("candidate was not evaluated: %s", output)
	}
}
