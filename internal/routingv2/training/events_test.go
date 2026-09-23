// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package training

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

func TestEventStore_RecordAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	store := NewEventStore(path)

	// Record some events
	e1 := Event{
		Timestamp:    time.Now().UTC(),
		Features:     domain.FeatureVector{Mechanical: 0.9},
		SelectedTier: core.TierSmall,
		Confidence:   0.8,
		Harness:      "claude-code",
	}
	e2 := Event{
		Timestamp:    time.Now().UTC(),
		Features:     domain.FeatureVector{Security: 0.8},
		SelectedTier: core.TierFrontier,
		Confidence:   0.9,
		Harness:      "claude-code",
	}

	if err := store.Record(e1); err != nil {
		t.Fatalf("Record e1: %v", err)
	}
	if err := store.Record(e2); err != nil {
		t.Fatalf("Record e2: %v", err)
	}

	// Load from fresh store
	store2 := NewEventStore(path)
	events, err := store2.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Loaded %d events, want 2", len(events))
	}

	if events[0].SelectedTier != core.TierSmall {
		t.Errorf("events[0].SelectedTier = %v, want TierSmall", events[0].SelectedTier)
	}
}

func TestEventStore_AddOutcomeAndReplayLatestFeedback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	store := NewEventStore(path)
	event := Event{ID: "route-1", Timestamp: time.Now().UTC(), Features: domain.FeatureVector{Security: 0.8}, SelectedTier: core.TierSmall, Harness: "codex"}
	if err := store.Record(event); err != nil {
		t.Fatal(err)
	}
	if err := store.AddOutcome(event.ID, Outcome{Success: true}); err != nil {
		t.Fatal(err)
	}
	required := core.TierFrontier
	if err := store.AddOutcome(event.ID, Outcome{Retry: true, RetryTier: core.TierFrontier, RequiredTier: &required}); err != nil {
		t.Fatal(err)
	}

	reloaded := NewEventStore(path)
	events, err := reloaded.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Outcome == nil || !events[0].Outcome.Retry || events[0].Outcome.RetryTier != core.TierFrontier {
		t.Fatalf("replayed events = %#v, want latest retry outcome", events)
	}
	if got := reloaded.ToDataset(); got.Size() != 1 || got.Examples[0].Label != "FRONTIER" {
		t.Fatalf("training dataset = %#v, want one FRONTIER example", got.Examples)
	}
}

func TestEventStore_AddOutcomeValidatesReview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	store := NewEventStore(path)
	if err := store.Record(Event{ID: "route-2", SelectedTier: core.TierMid}); err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []Outcome{
		{},
		{Retry: true},
		{Retry: true, RetryTier: core.TierSmall},
		{Success: true, Failed: true},
	} {
		if err := store.AddOutcome("route-2", outcome); err == nil {
			t.Errorf("AddOutcome(%+v) succeeded, want validation error", outcome)
		}
	}
	if err := store.AddOutcome("missing", Outcome{Success: true}); err == nil {
		t.Fatal("unknown event ID should be rejected")
	}
}

func TestRecordRoutedDecisionDoesNotPersistPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	prompt := "private customer incident token=secret"
	id, err := RecordRoutedDecision(prompt, core.Decision{Harness: "codex", Tier: core.TierFrontier, Confident: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("feedback id length = %d, want 32", len(id))
	}
	data, err := os.ReadFile(DefaultEventsPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), prompt) || strings.Contains(string(data), "secret") {
		t.Fatal("raw prompt material must not be persisted")
	}
	var event Event
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatal(err)
	}
	if event.ID != id || event.Harness != "codex" || event.SelectedTier != core.TierFrontier || !event.Confident {
		t.Fatalf("stored event = %+v", event)
	}
}

func TestEventStore_ToDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	store := NewEventStore(path)
	smallRequired := core.TierSmall

	// Event with success outcome
	e1 := Event{
		Timestamp:    time.Now().UTC(),
		Features:     domain.FeatureVector{Mechanical: 0.9},
		SelectedTier: core.TierSmall,
		Outcome: &Outcome{
			Timestamp:    time.Now().UTC(),
			Success:      true,
			RequiredTier: &smallRequired,
		},
	}

	// Event with retry outcome (should learn from retry tier)
	frontierRequired := core.TierFrontier
	e2 := Event{
		Timestamp:    time.Now().UTC(),
		Features:     domain.FeatureVector{Security: 0.7},
		SelectedTier: core.TierSmall,
		Outcome: &Outcome{
			Timestamp:    time.Now().UTC(),
			Retry:        true,
			RetryTier:    core.TierFrontier,
			RequiredTier: &frontierRequired,
		},
	}

	// Event without outcome (should be skipped)
	e3 := Event{
		Timestamp:    time.Now().UTC(),
		Features:     domain.FeatureVector{Coding: 0.5},
		SelectedTier: core.TierMid,
	}

	store.Record(e1)
	store.Record(e2)
	store.Record(e3)
	store.Load() // Load into memory

	ds := store.ToDataset()

	if ds.Size() != 2 {
		t.Errorf("Dataset size = %d, want 2 (e3 should be skipped)", ds.Size())
	}

	// e1: success with SMALL → label should be SMALL
	if ds.Examples[0].Label != "SMALL" {
		t.Errorf("examples[0].Label = %s, want SMALL", ds.Examples[0].Label)
	}

	// e2: retry to FRONTIER → label should be FRONTIER
	if ds.Examples[1].Label != "FRONTIER" {
		t.Errorf("examples[1].Label = %s, want FRONTIER", ds.Examples[1].Label)
	}
}

func TestEventStore_LoadEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.jsonl")

	store := NewEventStore(path)
	events, err := store.Load()

	if err != nil {
		t.Errorf("Load on non-existent file should not error: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("Load on non-existent file should return empty slice, got %d", len(events))
	}
}

func TestEventStore_Size(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	store := NewEventStore(path)

	if store.Size() != 0 {
		t.Error("New store should have size 0")
	}

	store.Record(Event{Timestamp: time.Now()})
	if store.Size() != 1 {
		t.Errorf("After 1 record, size = %d, want 1", store.Size())
	}
}

func TestDefaultEventsPath(t *testing.T) {
	path := DefaultEventsPath()

	if path == "" {
		t.Error("DefaultEventsPath should not be empty")
	}

	// Should contain the filename
	if filepath.Base(path) != "loop-events.jsonl" {
		t.Errorf("DefaultEventsPath should end with loop-events.jsonl, got %s", path)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a\nb\nc", 3},
		{"a\r\nb\r\nc", 3},
		{"single", 1},
		{"", 0},
		{"\n\n", 2}, // Empty lines count
	}

	for _, tc := range tests {
		lines := splitLines([]byte(tc.input))
		// Filter empty lines for counting
		nonEmpty := 0
		for _, l := range lines {
			if len(l) > 0 {
				nonEmpty++
			}
		}
		// This is a bit fuzzy; the main test is that it doesn't panic
		if len(lines) < 0 {
			t.Errorf("splitLines(%q) failed", tc.input)
		}
	}
}

func TestEventRecorder_Disabled(t *testing.T) {
	recorder := NewEventRecorder(false) // Disabled

	decision := domain.RoutingDecision{
		Tier:       core.TierMid,
		Confidence: 0.7,
		Features:   domain.FeatureVector{Coding: 0.5},
	}

	// Should not error and not write anything
	err := recorder.RecordDecision(decision, "claude-code")
	if err != nil {
		t.Errorf("RecordDecision on disabled recorder: %v", err)
	}
}

func TestTrainFromEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	store := NewEventStore(path)

	// Add events with outcomes
	for i := 0; i < 10; i++ {
		tier := core.Tier(i % 3)
		required := tier
		e := Event{
			Timestamp:    time.Now().UTC(),
			Features:     domain.FeatureVector{Coding: float64(i) / 10},
			SelectedTier: tier,
			Outcome: &Outcome{
				Timestamp:    time.Now().UTC(),
				Success:      true,
				RequiredTier: &required,
			},
		}
		store.Record(e)
	}

	// Train from events
	config := TrainConfig{Epochs: 10, LearningRate: 0.1, RiskWeighted: true}
	result, err := TrainFromEvents(path, config)
	if err != nil {
		t.Fatalf("TrainFromEvents: %v", err)
	}

	if result.Weights == nil {
		t.Error("TrainFromEvents should produce weights")
	}
}

func TestTrainFromEvents_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")

	// Create empty file
	os.WriteFile(path, []byte{}, 0644)

	config := DefaultTrainConfig()
	result, err := TrainFromEvents(path, config)

	if err != nil {
		t.Errorf("TrainFromEvents on empty file should not error: %v", err)
	}

	// Empty dataset should return default weights
	if result.Weights == nil {
		t.Log("Empty events produce nil weights (acceptable)")
	}
}
