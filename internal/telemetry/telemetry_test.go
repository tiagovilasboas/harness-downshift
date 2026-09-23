// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package telemetry_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/telemetry"
)

// tmpLog returns a fresh temp file path for a test event log.
func tmpLog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "events.jsonl")
}

// makeEvent constructs a minimal Event for testing.
func makeEvent(harness, complexity, verdict string, savings float64) telemetry.Event {
	return telemetry.Event{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Harness:          harness,
		Complexity:       complexity,
		FromModel:        "model-frontier",
		ToModel:          "model-small",
		Verdict:          verdict,
		EstimatedSavings: savings,
	}
}

// --- AppendTo / ReadEventsFrom round-trip ---

func TestAppendAndRead_RoundTrip(t *testing.T) {
	path := tmpLog(t)
	ev := makeEvent("claude-code", "TRIVIAL", "DOWNSHIFT", 0.85)

	if err := telemetry.AppendTo(path, ev); err != nil {
		t.Fatalf("AppendTo: %v", err)
	}
	got, err := telemetry.ReadEventsFrom(path)
	if err != nil {
		t.Fatalf("ReadEventsFrom: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(got))
	}
	if got[0].Harness != ev.Harness {
		t.Errorf("Harness = %q, want %q", got[0].Harness, ev.Harness)
	}
	if got[0].Complexity != ev.Complexity {
		t.Errorf("Complexity = %q, want %q", got[0].Complexity, ev.Complexity)
	}
	if got[0].Verdict != ev.Verdict {
		t.Errorf("Verdict = %q, want %q", got[0].Verdict, ev.Verdict)
	}
	if math.Abs(got[0].EstimatedSavings-ev.EstimatedSavings) > 1e-9 {
		t.Errorf("EstimatedSavings = %f, want %f", got[0].EstimatedSavings, ev.EstimatedSavings)
	}
}

func TestAppendTo_UsesPrivateFilePermissions(t *testing.T) {
	path := tmpLog(t)
	if err := telemetry.AppendTo(path, makeEvent("claude-code", "TRIVIAL", "DOWNSHIFT", 0.8)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("event log permissions = %04o, want 0600", got)
	}
}

func TestAppendMultiple_AllPresent(t *testing.T) {
	path := tmpLog(t)
	for i := 0; i < 5; i++ {
		ev := makeEvent("claude-code", "TRIVIAL", "DOWNSHIFT", 0.8)
		if err := telemetry.AppendTo(path, ev); err != nil {
			t.Fatalf("AppendTo[%d]: %v", i, err)
		}
	}
	got, err := telemetry.ReadEventsFrom(path)
	if err != nil {
		t.Fatalf("ReadEventsFrom: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("len(events) = %d, want 5", len(got))
	}
}

// --- Missing file ---

func TestReadEventsFrom_MissingFile_ReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.jsonl")
	got, err := telemetry.ReadEventsFrom(path)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d events", len(got))
	}
}

// --- Corrupted JSONL ---

func TestReadEventsFrom_CorruptedLines_SkippedGracefully(t *testing.T) {
	path := tmpLog(t)

	good := makeEvent("claude-code", "TRIVIAL", "DOWNSHIFT", 0.8)
	if err := telemetry.AppendTo(path, good); err != nil {
		t.Fatal(err)
	}

	// Inject a corrupted line directly.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fmt.Fprintln(f, `{not valid json at all`)
	fmt.Fprintln(f, `{"incomplete":`)
	f.Close()

	// Append a second good event after the corrupt lines.
	if err := telemetry.AppendTo(path, good); err != nil {
		t.Fatal(err)
	}

	got, err := telemetry.ReadEventsFrom(path)
	if err != nil {
		t.Fatalf("ReadEventsFrom: %v", err)
	}
	// Corrupted lines are skipped; the two good events survive.
	if len(got) != 2 {
		t.Errorf("expected 2 good events, got %d", len(got))
	}
}

// --- FilterByDays ---

func TestFilterByDays_ZeroReturnsAll(t *testing.T) {
	events := []telemetry.Event{
		makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.8),
		makeEvent("cc", "COMPLEX", "OK", 0),
	}
	got := telemetry.FilterByDays(events, 0)
	if len(got) != 2 {
		t.Errorf("days=0 should return all events, got %d", len(got))
	}
}

func TestFilterByDays_FiltersOldEvents(t *testing.T) {
	old := telemetry.Event{
		Timestamp:  time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339),
		Harness:    "cc",
		Complexity: "TRIVIAL",
		Verdict:    "DOWNSHIFT",
	}
	recent := makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.8)

	got := telemetry.FilterByDays([]telemetry.Event{old, recent}, 1)
	if len(got) != 1 {
		t.Errorf("expected 1 recent event, got %d", len(got))
	}
}

func TestFilterByDays_NegativeReturnsAll(t *testing.T) {
	events := []telemetry.Event{
		makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.8),
	}
	got := telemetry.FilterByDays(events, -7)
	if len(got) != 1 {
		t.Errorf("negative days should return all events, got %d", len(got))
	}
}

// --- Aggregate ---

func TestAggregate_CountsVerdicts(t *testing.T) {
	events := []telemetry.Event{
		makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.8),
		makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.8),
		makeEvent("cc", "COMPLEX", "UPSHIFT", 0),
		makeEvent("cc", "MEDIUM", "OK", 0),
		makeEvent("cc", "MEDIUM", "UNKNOWN", 0),
	}
	s := telemetry.Aggregate(events)
	if s.Total != 5 {
		t.Errorf("Total = %d, want 5", s.Total)
	}
	if s.Downshifted != 2 {
		t.Errorf("Downshifted = %d, want 2", s.Downshifted)
	}
	if s.Upshifted != 1 {
		t.Errorf("Upshifted = %d, want 1", s.Upshifted)
	}
	if s.OK != 1 {
		t.Errorf("OK = %d, want 1", s.OK)
	}
	if s.Unknown != 1 {
		t.Errorf("Unknown = %d, want 1", s.Unknown)
	}
}

func TestAggregate_NormSavings_WithDownshift(t *testing.T) {
	// One event with 50% savings: routed cost = 0.5, baseline = 1.0.
	events := []telemetry.Event{
		makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.5),
	}
	s := telemetry.Aggregate(events)
	saved, frac := s.NormSaved()
	if math.Abs(saved-0.5) > 1e-9 {
		t.Errorf("saved = %f, want 0.5", saved)
	}
	if math.Abs(frac-0.5) > 1e-9 {
		t.Errorf("frac = %f, want 0.5", frac)
	}
}

func TestAggregate_NormSavings_NoDownshift(t *testing.T) {
	events := []telemetry.Event{
		makeEvent("cc", "COMPLEX", "OK", 0),
	}
	s := telemetry.Aggregate(events)
	saved, frac := s.NormSaved()
	if saved != 0 {
		t.Errorf("saved = %f, want 0 (no downshift)", saved)
	}
	if frac != 0 {
		t.Errorf("frac = %f, want 0 (no downshift)", frac)
	}
}

func TestAggregate_EmptySlice(t *testing.T) {
	s := telemetry.Aggregate(nil)
	if s.Total != 0 {
		t.Errorf("Total = %d, want 0", s.Total)
	}
	saved, frac := s.NormSaved()
	if saved != 0 || frac != 0 {
		t.Errorf("empty aggregate should have zero savings, got %f %f", saved, frac)
	}
}

// --- PrintStats ---

func TestPrintStats_UnitMode_NoUSDClaim(t *testing.T) {
	events := []telemetry.Event{
		makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.8),
	}
	var buf strings.Builder
	telemetry.PrintStats(events, telemetry.StatsOptions{Days: 30}, &buf)
	out := buf.String()

	if strings.Contains(out, "$") {
		t.Error("unit mode must not show dollar signs")
	}
	if !strings.Contains(out, "Normalised") {
		t.Error("unit mode should label savings as 'Normalised'")
	}
	if !strings.Contains(out, "Not real dollars") {
		t.Error("unit mode should disclaim real-dollar interpretation")
	}
}

func TestPrintStats_DollarMode_ShowsUSD(t *testing.T) {
	events := []telemetry.Event{
		makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.5),
	}
	var buf strings.Builder
	telemetry.PrintStats(events, telemetry.StatsOptions{Days: 30, CostPerUnit: 0.10}, &buf)
	out := buf.String()

	if !strings.Contains(out, "$") {
		t.Error("dollar mode should show dollar signs")
	}
	if !strings.Contains(out, "--cost-per-unit=") {
		t.Error("dollar mode should echo the rate used")
	}
}

func TestPrintStats_AllTimeLabel(t *testing.T) {
	events := []telemetry.Event{makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.5)}
	var buf strings.Builder
	telemetry.PrintStats(events, telemetry.StatsOptions{Days: 0}, &buf)
	if !strings.Contains(buf.String(), "All time") {
		t.Error("days=0 should show 'All time' label")
	}
}

// --- Concurrent writes ---

func TestAppendTo_ConcurrentWrites_NoDataLoss(t *testing.T) {
	path := tmpLog(t)
	const goroutines = 20
	const eventsEach = 10

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < eventsEach; i++ {
				ev := makeEvent("cc", "TRIVIAL", "DOWNSHIFT", 0.8)
				if err := telemetry.AppendTo(path, ev); err != nil {
					t.Errorf("AppendTo: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	got, err := telemetry.ReadEventsFrom(path)
	if err != nil {
		t.Fatalf("ReadEventsFrom: %v", err)
	}
	want := goroutines * eventsEach
	if len(got) != want {
		t.Errorf("concurrent writes: got %d events, want %d", len(got), want)
	}
}
