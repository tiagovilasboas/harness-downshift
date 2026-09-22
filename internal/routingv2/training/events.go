// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package training provides event collection for personalized routing.
//
// The event collection system is designed for privacy-first learning:
// - Only features (derived signals) are stored, never the raw prompts
// - Events capture features + routing decision + outcome
// - Training happens locally from events: `downshift train --from-events`
//
// Flow:
// 1. Runtime: router extracts features, makes decision, records event
// 2. User approves/rejects/retries → outcome is appended
// 3. Offline: user runs `downshift train --from-events` to update weights
// 4. User compares current vs candidate weights with `downshift benchmark --compare`
// 5. User manually activates new weights by copying to ~/.harness-downshift/weights.json
package training

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// Event represents a single routing event for later training.
// Note: no prompt is stored — only derived features. This is intentional.
type Event struct {
	Timestamp   time.Time           `json:"timestamp"`
	Features    domain.FeatureVector `json:"features"`
	SelectedTier core.Tier           `json:"selected_tier"`
	Confidence  float64             `json:"confidence"`
	Harness     string              `json:"harness"`

	// Outcome — set after task completion (may be empty initially)
	Outcome *Outcome `json:"outcome,omitempty"`
}

// Outcome captures what happened after routing.
type Outcome struct {
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`    // Task completed without escalation
	Retry     bool      `json:"retry"`      // Needed to escalate to higher tier
	RetryTier core.Tier `json:"retry_tier,omitempty"` // The tier used on retry
}

// EventStore manages event persistence for training.
type EventStore struct {
	path   string
	events []Event
	mu     sync.RWMutex
}

// DefaultEventsPath returns the default path for event storage.
func DefaultEventsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "events.jsonl"
	}
	return filepath.Join(home, ".harness-downshift", "events.jsonl")
}

// NewEventStore creates an event store at the given path.
// Events are stored as JSON Lines (one JSON object per line).
func NewEventStore(path string) *EventStore {
	return &EventStore{
		path:   path,
		events: nil,
	}
}

// Record adds a new event to the store.
// The event is appended to the file immediately (append-only).
func (s *EventStore) Record(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, e)

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Append to file
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	_, err = f.Write(append(data, '\n'))
	return err
}

// Load reads all events from the store.
func (s *EventStore) Load() ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var events []Event
	lines := splitLines(data)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // Skip malformed lines
		}
		events = append(events, e)
	}

	s.events = events
	return events, nil
}

// splitLines splits data by newlines, handling both \n and \r\n.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			end := i
			if end > start && data[end-1] == '\r' {
				end--
			}
			lines = append(lines, data[start:end])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// Size returns the number of events in memory.
func (s *EventStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// ToDataset converts events with outcomes into a training dataset.
// Events without outcomes are skipped.
// When an event has retry=true, the correct label is retry_tier (what actually worked).
// When success=true, the selected_tier is the correct label.
func (s *EventStore) ToDataset() *Dataset {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var examples []Example
	for _, e := range s.events {
		if e.Outcome == nil {
			continue // Skip events without outcome
		}

		var label core.Tier
		if e.Outcome.Retry && e.Outcome.RetryTier > e.SelectedTier {
			// The selected tier wasn't enough — the correct tier is what worked
			label = e.Outcome.RetryTier
		} else if e.Outcome.Success {
			// Success with selected tier — it was correct (or higher than needed)
			label = e.SelectedTier
		} else {
			// Failed without retry — hard to learn from, skip
			continue
		}

		examples = append(examples, Example{
			Prompt:   "", // No prompt stored — features only
			Label:    tierLabel(label),
			Features: e.Features,
		})
	}

	return &Dataset{Examples: examples}
}

// tierLabel converts a Tier to its string label.
func tierLabel(t core.Tier) string {
	switch t {
	case core.TierSmall:
		return "SMALL"
	case core.TierMid:
		return "MID"
	default:
		return "FRONTIER"
	}
}

// EventRecorder is a helper for recording events from routing decisions.
type EventRecorder struct {
	store   *EventStore
	enabled bool
}

// NewEventRecorder creates a recorder that writes to the default path.
// Set enabled=true to actually record; false for no-op (useful for testing).
func NewEventRecorder(enabled bool) *EventRecorder {
	path := DefaultEventsPath()
	return &EventRecorder{
		store:   NewEventStore(path),
		enabled: enabled,
	}
}

// RecordDecision records a routing decision as an event.
// The outcome should be added later via AddOutcome when the task completes.
func (r *EventRecorder) RecordDecision(decision domain.RoutingDecision, harness string) error {
	if !r.enabled {
		return nil
	}

	e := Event{
		Timestamp:    time.Now().UTC(),
		Features:     decision.Features,
		SelectedTier: decision.Tier,
		Confidence:   decision.Confidence,
		Harness:      harness,
	}

	return r.store.Record(e)
}

// TrainFromEvents loads events and trains a new set of weights.
func TrainFromEvents(eventsPath string, config TrainConfig) (TrainResult, error) {
	store := NewEventStore(eventsPath)
	if _, err := store.Load(); err != nil {
		return TrainResult{}, err
	}

	ds := store.ToDataset()
	if ds.Size() == 0 {
		return TrainResult{}, nil
	}

	train, val := ds.Split(0.2)
	return Train(train, val, config), nil
}
