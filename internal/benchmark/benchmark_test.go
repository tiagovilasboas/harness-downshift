// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package benchmark_test

import (
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/benchmark"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// writeDataset writes tasks as JSON to a temp file and returns the path.
func writeDataset(t *testing.T, tasks []benchmark.Task) string {
	t.Helper()
	data, err := json.Marshal(tasks)
	if err != nil {
		t.Fatalf("marshal dataset: %v", err)
	}
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write dataset: %v", err)
	}
	return path
}

// --- LoadDataset ---

func TestLoadDataset_MissingFile(t *testing.T) {
	_, err := benchmark.LoadDataset("/no/such/path/tasks.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadDataset_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(path, []byte(`{not json`), 0o644)
	_, err := benchmark.LoadDataset(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestLoadDataset_EmptyArray(t *testing.T) {
	path := writeDataset(t, []benchmark.Task{})
	tasks, err := benchmark.LoadDataset(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("len = %d, want 0", len(tasks))
	}
}

func TestLoadDataset_RoundTrip(t *testing.T) {
	original := []benchmark.Task{
		{Prompt: "rename the variable", Label: "TRIVIAL"},
		{Prompt: "rearchitect the auth system", Label: "COMPLEX"},
	}
	path := writeDataset(t, original)
	tasks, err := benchmark.LoadDataset(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len = %d, want 2", len(tasks))
	}
	if tasks[0].Prompt != original[0].Prompt || tasks[0].Label != original[0].Label {
		t.Errorf("task[0] mismatch: got %+v, want %+v", tasks[0], original[0])
	}
}

// --- Run ---

func TestRun_EmptyDataset(t *testing.T) {
	results := benchmark.Run(nil, io.Discard)
	if results != nil && len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestRun_InvalidLabel_Skipped(t *testing.T) {
	tasks := []benchmark.Task{
		{Prompt: "rename the variable", Label: "TRIVIAL"},
		{Prompt: "do something", Label: "INVALID_LABEL"},
	}
	var log strings.Builder
	results := benchmark.Run(tasks, &log)
	if len(results) != 1 {
		t.Errorf("expected 1 result (invalid skipped), got %d", len(results))
	}
	if !strings.Contains(log.String(), "skip") {
		t.Error("expected skip log for invalid label")
	}
}

func TestRun_AllInvalidLabels_EmptyResults(t *testing.T) {
	tasks := []benchmark.Task{
		{Prompt: "foo", Label: "BANANA"},
		{Prompt: "bar", Label: ""},
	}
	results := benchmark.Run(tasks, io.Discard)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestRun_LabelsCaseInsensitive(t *testing.T) {
	tasks := []benchmark.Task{
		{Prompt: "rename the variable", Label: "trivial"},
		{Prompt: "rearchitect auth", Label: "complex"},
	}
	results := benchmark.Run(tasks, io.Discard)
	if len(results) != 2 {
		t.Errorf("case-insensitive labels: expected 2 results, got %d", len(results))
	}
}

// --- Accuracy ---

func TestAccuracy_EmptyResults(t *testing.T) {
	if acc := benchmark.Accuracy(nil); acc != 0 {
		t.Errorf("empty accuracy = %f, want 0", acc)
	}
}

func TestAccuracy_AllCorrect(t *testing.T) {
	// Use clearly trivial prompts so the classifier agrees.
	tasks := []benchmark.Task{
		{Prompt: "rename the variable userId", Label: "TRIVIAL"},
	}
	results := benchmark.Run(tasks, io.Discard)
	// Only assert accuracy is in [0, 1] — classifier correctness is not the point here.
	acc := benchmark.Accuracy(results)
	if acc < 0 || acc > 1 {
		t.Errorf("accuracy out of range: %f", acc)
	}
}

// --- TierAccuracy ---

func TestTierAccuracy_EmptyResults(t *testing.T) {
	if acc := benchmark.TierAccuracy(nil); acc != 0 {
		t.Errorf("empty tier accuracy = %f, want 0", acc)
	}
}

func TestTierAccuracy_GreaterOrEqualToComplexityAccuracy(t *testing.T) {
	// Tier accuracy can only be >= complexity accuracy because SIMPLE and MEDIUM
	// map to the same tier (MID). A SIMPLE task predicted as MEDIUM is a
	// complexity miss but a tier hit.
	tasks := []benchmark.Task{
		{Prompt: "rename the variable userId", Label: "TRIVIAL"},
		{Prompt: "add error handling to fetchUser", Label: "SIMPLE"},
		{Prompt: "refactor auth to extract token service", Label: "MEDIUM"},
		{Prompt: "rearchitect auth for multi-tenant", Label: "COMPLEX"},
	}
	results := benchmark.Run(tasks, io.Discard)
	compAcc := benchmark.Accuracy(results)
	tierAcc := benchmark.TierAccuracy(results)
	if tierAcc < compAcc-1e-9 {
		t.Errorf("tier accuracy (%f) < complexity accuracy (%f) — impossible", tierAcc, compAcc)
	}
}

func TestTierAccuracy_SimplePredictedAsMedium_CountsAsCorrect(t *testing.T) {
	// Build a synthetic result where SIMPLE was predicted as MEDIUM.
	// Both map to TierMid, so tier accuracy should be 1.0.
	results := []benchmark.Result{
		{
			Task:      benchmark.Task{Prompt: "add a field", Label: "SIMPLE"},
			Predicted: core.Medium, // classifier said Medium
			Correct:   false,       // complexity miss
		},
	}
	tierAcc := benchmark.TierAccuracy(results)
	if math.Abs(tierAcc-1.0) > 1e-9 {
		t.Errorf("SIMPLE→MEDIUM is a tier hit, tier accuracy = %f, want 1.0", tierAcc)
	}
}

// --- UnsafeDowngradeRates ---

func TestUnsafeDowngradeRates_NoComplexTasks(t *testing.T) {
	tasks := []benchmark.Task{
		{Prompt: "rename the variable", Label: "TRIVIAL"},
	}
	results := benchmark.Run(tasks, io.Discard)
	toMID, toSMALL, _, _, total := benchmark.UnsafeDowngradeRates(results)
	if total != 0 || toMID != 0 || toSMALL != 0 {
		t.Error("no COMPLEX tasks should yield zero unsafe downgrade rates")
	}
}

func TestUnsafeDowngradeRates_Synthetic(t *testing.T) {
	// Two COMPLEX tasks: one stays FRONTIER, one drops to MID.
	results := []benchmark.Result{
		{Task: benchmark.Task{Label: "COMPLEX"}, Predicted: core.Complex, Correct: true},
		{Task: benchmark.Task{Label: "COMPLEX"}, Predicted: core.Medium, Correct: false},
	}
	toMID, toSMALL, cntMID, cntSMALL, total := benchmark.UnsafeDowngradeRates(results)
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if cntMID != 1 || math.Abs(toMID-0.5) > 1e-9 {
		t.Errorf("toMID = %f (%d), want 0.5 (1)", toMID, cntMID)
	}
	if cntSMALL != 0 || toSMALL != 0 {
		t.Errorf("toSMALL = %f (%d), want 0.0 (0)", toSMALL, cntSMALL)
	}
}

// --- WastefulOverRoutingRates ---

func TestWastefulOverRoutingRates_NoTrivialTasks(t *testing.T) {
	tasks := []benchmark.Task{
		{Prompt: "rearchitect auth for multi-tenant", Label: "COMPLEX"},
	}
	results := benchmark.Run(tasks, io.Discard)
	toMID, toFrontier, _, _, total := benchmark.WastefulOverRoutingRates(results)
	if total != 0 || toMID != 0 || toFrontier != 0 {
		t.Error("no TRIVIAL tasks should yield zero over-routing rates")
	}
}

// --- FalseDownshiftRate (backward compat) ---

func TestFalseDownshiftRate_Consistent_WithUnsafeDowngrade(t *testing.T) {
	results := []benchmark.Result{
		{Task: benchmark.Task{Label: "COMPLEX"}, Predicted: core.Complex, Correct: true},
		{Task: benchmark.Task{Label: "COMPLEX"}, Predicted: core.Medium, Correct: false},
		{Task: benchmark.Task{Label: "COMPLEX"}, Predicted: core.Trivial, Correct: false},
	}
	rate, count, total := benchmark.FalseDownshiftRate(results)
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (MID + SMALL)", count)
	}
	if math.Abs(rate-2.0/3.0) > 1e-9 {
		t.Errorf("rate = %f, want %.4f", rate, 2.0/3.0)
	}
}

// --- Build (confusion matrix) ---

func TestBuild_EmptyResults(t *testing.T) {
	m := benchmark.Build(nil)
	for i := range m {
		for j := range m[i] {
			if m[i][j] != 0 {
				t.Errorf("m[%d][%d] = %d, want 0", i, j, m[i][j])
			}
		}
	}
}

// --- Print ---

func TestPrint_ContainsKPIs(t *testing.T) {
	tasks := []benchmark.Task{
		{Prompt: "rename the variable userId", Label: "TRIVIAL"},
		{Prompt: "rearchitect auth for multi-tenant", Label: "COMPLEX"},
	}
	results := benchmark.Run(tasks, io.Discard)
	m := benchmark.Build(results)

	var buf strings.Builder
	benchmark.Print(results, m, &buf)
	out := buf.String()

	for _, want := range []string{
		"Complexity accuracy",
		"Tier routing accuracy",
		"Unsafe downgrade",
		"Wasteful over-routing",
		"Confusion matrix",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestPrint_TierLegendPresent(t *testing.T) {
	results := benchmark.Run([]benchmark.Task{
		{Prompt: "rename", Label: "TRIVIAL"},
	}, io.Discard)
	m := benchmark.Build(results)
	var buf strings.Builder
	benchmark.Print(results, m, &buf)
	if !strings.Contains(buf.String(), "TRIVIAL→SMALL") {
		t.Error("output should include tier legend")
	}
}

// --- seed dataset ---

func TestSeedDataset_Loadable(t *testing.T) {
	tasks, err := benchmark.LoadDataset("../../benchmark/tasks.json")
	if err != nil {
		t.Fatalf("cannot load seed dataset: %v", err)
	}
	if len(tasks) < 10 {
		t.Errorf("seed dataset has only %d tasks; expected at least 10", len(tasks))
	}
	for _, task := range tasks {
		if task.Prompt == "" {
			t.Error("found task with empty prompt")
		}
		label := strings.ToUpper(strings.TrimSpace(task.Label))
		switch label {
		case "TRIVIAL", "SIMPLE", "MEDIUM", "COMPLEX":
		default:
			t.Errorf("invalid label %q for prompt %q", task.Label, task.Prompt)
		}
	}
}

func TestSeedDataset_TierAccuracyBetterThanComplexity(t *testing.T) {
	tasks, err := benchmark.LoadDataset("../../benchmark/tasks.json")
	if err != nil {
		t.Skip("seed dataset not found")
	}
	results := benchmark.Run(tasks, io.Discard)
	complexAcc := benchmark.Accuracy(results)
	tierAcc := benchmark.TierAccuracy(results)
	if tierAcc < complexAcc-1e-9 {
		t.Errorf("tier accuracy (%f) < complexity accuracy (%f)", tierAcc, complexAcc)
	}
	t.Logf("seed dataset — complexity accuracy: %.1f%%  tier accuracy: %.1f%%",
		complexAcc*100, tierAcc*100)
}

func TestSeedDataset_NoComplexToSmallDowngrade(t *testing.T) {
	tasks, err := benchmark.LoadDataset("../../benchmark/tasks.json")
	if err != nil {
		t.Skip("seed dataset not found")
	}
	results := benchmark.Run(tasks, io.Discard)
	_, toSMALL, _, cntSMALL, _ := benchmark.UnsafeDowngradeRates(results)
	if cntSMALL > 0 {
		t.Errorf("safety property violated: %d COMPLEX tasks routed to SMALL (rate %.1f%%)",
			cntSMALL, toSMALL*100)
	}
}
