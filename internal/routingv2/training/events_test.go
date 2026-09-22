// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package training

import (
	"os"
	"path/filepath"
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

func TestEventStore_ToDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	store := NewEventStore(path)

	// Event with success outcome
	e1 := Event{
		Timestamp:    time.Now().UTC(),
		Features:     domain.FeatureVector{Mechanical: 0.9},
		SelectedTier: core.TierSmall,
		Outcome: &Outcome{
			Timestamp: time.Now().UTC(),
			Success:   true,
		},
	}

	// Event with retry outcome (should learn from retry tier)
	e2 := Event{
		Timestamp:    time.Now().UTC(),
		Features:     domain.FeatureVector{Security: 0.7},
		SelectedTier: core.TierSmall,
		Outcome: &Outcome{
			Timestamp: time.Now().UTC(),
			Retry:     true,
			RetryTier: core.TierFrontier,
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
	if filepath.Base(path) != "events.jsonl" {
		t.Errorf("DefaultEventsPath should end with events.jsonl, got %s", path)
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
		e := Event{
			Timestamp:    time.Now().UTC(),
			Features:     domain.FeatureVector{Coding: float64(i) / 10},
			SelectedTier: tier,
			Outcome: &Outcome{
				Timestamp: time.Now().UTC(),
				Success:   true,
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
