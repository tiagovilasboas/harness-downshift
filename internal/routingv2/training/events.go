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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/extractor"
)

// Event represents a single routing event for later training.
// Note: no prompt is stored — only derived features. This is intentional.
type Event struct {
	RecordType   string               `json:"record_type,omitempty"`
	ID           string               `json:"id,omitempty"`
	EventID      string               `json:"event_id,omitempty"`
	Timestamp    time.Time            `json:"timestamp"`
	Features     domain.FeatureVector `json:"features"`
	SelectedTier core.Tier            `json:"selected_tier"`
	Confidence   float64              `json:"confidence,omitempty"` // Probability from a statistical router, when available
	Confident    bool                 `json:"confident,omitempty"`  // Binary signal from the deterministic router
	Harness      string               `json:"harness"`

	// Outcome — attached after engineer review (may be empty initially)
	Outcome *Outcome `json:"outcome,omitempty"`
}

// Outcome captures what happened after routing.
type Outcome struct {
	Timestamp    time.Time  `json:"timestamp"`
	Success      bool       `json:"success"`                 // Task completed without escalation
	Retry        bool       `json:"retry"`                   // Needed to escalate to higher tier
	Failed       bool       `json:"failed,omitempty"`        // Reviewed, but did not produce a usable result
	RetryTier    core.Tier  `json:"retry_tier,omitempty"`    // The tier used on retry
	RequiredTier *core.Tier `json:"required_tier,omitempty"` // Engineer-reviewed minimum tier label
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
	return filepath.Join(home, ".harness-downshift", "loop-events.jsonl")
}

// RecordRoutedDecision stores only derived features and routing metadata. The
// raw task prompt is intentionally discarded before persistence.
func RecordRoutedDecision(prompt string, decision core.Decision) (string, error) {
	if strings.TrimSpace(prompt) == "" || decision.Harness == "" {
		return "", nil
	}
	var rawID [16]byte
	if _, err := rand.Read(rawID[:]); err != nil {
		return "", fmt.Errorf("creating feedback ID: %w", err)
	}
	id := hex.EncodeToString(rawID[:])
	event := Event{
		RecordType:   "decision",
		ID:           id,
		Timestamp:    time.Now().UTC(),
		Features:     extractor.Extract(prompt),
		SelectedTier: decision.Tier,
		Confident:    decision.Confident,
		Harness:      decision.Harness,
	}
	if err := NewEventStore(DefaultEventsPath()).Record(event); err != nil {
		return "", err
	}
	return id, nil
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

	e.RecordType = "decision"

	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Append to file
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	if _, err = f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	s.events = append(s.events, e)
	return nil
}

// AddOutcome appends reviewed feedback for a known routing ID. Repeated
// feedback is allowed so an engineer can correct an earlier review; the most
// recent review is the one used for training.
func (s *EventStore) AddOutcome(eventID string, outcome Outcome) error {
	if strings.TrimSpace(eventID) == "" {
		return fmt.Errorf("feedback ID is required")
	}
	if err := validateOutcome(outcome); err != nil {
		return err
	}
	events, err := s.Load()
	if err != nil {
		return err
	}
	var found *Event
	for i := range events {
		if events[i].ID == eventID {
			found = &events[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("unknown feedback ID %q", eventID)
	}
	if outcome.Retry && outcome.RetryTier <= found.SelectedTier {
		return fmt.Errorf("retry tier must be stronger than selected tier %s", found.SelectedTier)
	}
	if outcome.RequiredTier != nil && (*outcome.RequiredTier < core.TierSmall || *outcome.RequiredTier > core.TierFrontier) {
		return fmt.Errorf("required tier must be SMALL, MID, or FRONTIER")
	}
	if outcome.Timestamp.IsZero() {
		outcome.Timestamp = time.Now().UTC()
	}
	if err := s.appendRecord(Event{RecordType: "outcome", EventID: eventID, Outcome: &outcome}); err != nil {
		return err
	}
	return nil
}

func validateOutcome(outcome Outcome) error {
	count := 0
	if outcome.Success {
		count++
	}
	if outcome.Retry {
		count++
	}
	if outcome.Failed {
		count++
	}
	if count != 1 {
		return fmt.Errorf("outcome must be exactly one of success, retry, or failed")
	}
	if outcome.Retry && outcome.RetryTier != core.TierMid && outcome.RetryTier != core.TierFrontier {
		return fmt.Errorf("retry tier must be MID or FRONTIER")
	}
	if !outcome.Retry && outcome.RetryTier != 0 {
		return fmt.Errorf("retry tier is only valid with a retry outcome")
	}
	if outcome.RequiredTier != nil && (*outcome.RequiredTier < core.TierSmall || *outcome.RequiredTier > core.TierFrontier) {
		return fmt.Errorf("required tier must be SMALL, MID, or FRONTIER")
	}
	return nil
}

func (s *EventStore) appendRecord(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if e.RecordType == "outcome" {
		for i := range s.events {
			if s.events[i].ID == e.EventID {
				s.events[i].Outcome = e.Outcome
				break
			}
		}
	}
	return nil
}

// Load reads all events from the store.
func (s *EventStore) Load() ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.events = nil
			return nil, nil
		}
		return nil, err
	}

	var events []Event
	outcomes := make(map[string]*Outcome)
	lines := splitLines(data)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // Skip malformed lines
		}
		if e.RecordType == "outcome" {
			if e.EventID != "" && e.Outcome != nil && validateOutcome(*e.Outcome) == nil {
				outcomes[e.EventID] = e.Outcome
			}
			continue
		}
		if e.SelectedTier < core.TierSmall || e.SelectedTier > core.TierFrontier ||
			e.Confidence < 0 || e.Confidence > 1 || math.IsNaN(e.Confidence) || math.IsInf(e.Confidence, 0) ||
			validateFeatures(e.Features) != nil {
			continue
		}
		events = append(events, e)
	}
	for i := range events {
		if outcome, ok := outcomes[events[i].ID]; ok {
			if validateOutcome(*outcome) == nil && (!outcome.Retry || outcome.RetryTier > events[i].SelectedTier) {
				events[i].Outcome = outcome
			}
		}
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

// ToDataset converts engineer-reviewed minimum-tier labels into training data.
// Outcomes without RequiredTier are useful for quality stats but are not
// sufficient to establish the minimum capable tier.
func (s *EventStore) ToDataset() *Dataset {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var examples []Example
	for _, e := range s.events {
		if e.Outcome == nil || validateOutcome(*e.Outcome) != nil ||
			(e.Outcome.Retry && e.Outcome.RetryTier <= e.SelectedTier) || e.Outcome.RequiredTier == nil {
			continue // Skip events without outcome
		}
		// Success proves sufficiency, not that the selected tier was the minimum.
		label := *e.Outcome.RequiredTier

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
