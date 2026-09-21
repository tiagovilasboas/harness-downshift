// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package telemetry records routing decisions to a local JSONL file and
// renders aggregated stats. All data stays on the user's machine — nothing
// is sent to any external service.
//
// Event log location: ~/.harness-downshift/events.jsonl
// Each line is one JSON object written atomically (append + sync).
package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// Event is one routing decision persisted to the event log.
type Event struct {
	Timestamp        string  `json:"timestamp"`          // RFC3339
	Harness          string  `json:"harness"`            // e.g. "claude-code"
	Complexity       string  `json:"complexity"`         // TRIVIAL|SIMPLE|MEDIUM|COMPLEX
	FromModel        string  `json:"from"`               // model that would have run
	ToModel          string  `json:"to"`                 // model that will run
	Verdict          string  `json:"verdict"`            // DOWNSHIFT|UPSHIFT|OK|UNKNOWN
	EstimatedSavings float64 `json:"estimated_savings"`  // fraction 0–1 (0 = no saving)
}

// FromDecision builds an Event from a core.Decision. from is the model that
// was active before routing; to is the model we routed to.
func FromDecision(d core.Decision) Event {
	from := d.CurrentModel.ID
	if from == "" {
		from = d.Model.ID // unknown current → use recommended as baseline
	}
	return Event{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Harness:          d.Harness,
		Complexity:       d.Complexity.String(),
		FromModel:        from,
		ToModel:          d.Model.ID,
		Verdict:          d.Verdict.String(),
		EstimatedSavings: d.Savings,
	}
}

// defaultEventPath returns ~/.harness-downshift/events.jsonl.
func defaultEventPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".harness-downshift", "events.jsonl")
}

var mu sync.Mutex

// Record appends ev to the default event log. Errors are silently dropped —
// a telemetry failure must never interrupt a hook execution.
func Record(ev Event) {
	_ = AppendTo(defaultEventPath(), ev)
}

// AppendTo appends ev to the given path, creating the file and parent
// directories as needed. Returns any write error.
func AppendTo(path string, ev Event) error {
	mu.Lock()
	defer mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

// Stats is the aggregated view of all recorded events.
type Stats struct {
	Total       int
	Downshifted int
	Upshifted   int
	OK          int
	Unknown     int

	// EstimatedBaselineCost is what the sessions would have cost without routing,
	// computed as if every subagent ran on the from-model at a synthetic
	// token volume (1 000 input + 200 output per event).
	EstimatedBaselineCost float64
	EstimatedRoutedCost   float64

	// ByComplexity counts decisions per complexity class.
	ByComplexity map[string]int
}

// Saved returns the absolute dollar saving and the fraction.
func (s Stats) Saved() (float64, float64) {
	saved := s.EstimatedBaselineCost - s.EstimatedRoutedCost
	if s.EstimatedBaselineCost <= 0 {
		return 0, 0
	}
	return saved, saved / s.EstimatedBaselineCost
}

// syntheticTokenCost estimates the cost of one event given a model's per-1M
// rates. We assume 1 000 input tokens + 200 output tokens per subagent call —
// a rough but consistent proxy for comparing baseline vs routed cost.
func syntheticTokenCost(inputPerM, outputPerM float64) float64 {
	const inputTokens = 1_000
	const outputTokens = 200
	return (inputPerM/1e6)*inputTokens + (outputPerM/1e6)*outputTokens
}

// ReadEvents reads all events from the default event log. Returns an empty
// slice (not an error) when the file does not exist.
func ReadEvents() ([]Event, error) {
	return ReadEventsFrom(defaultEventPath())
}

// ReadEventsFrom reads all events from path.
func ReadEventsFrom(path string) ([]Event, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseEvents(f)
}

func parseEvents(r io.Reader) ([]Event, error) {
	var events []Event
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // skip malformed lines
		}
		events = append(events, ev)
	}
	return events, sc.Err()
}

// Aggregate computes Stats from a slice of events.
func Aggregate(events []Event) Stats {
	s := Stats{ByComplexity: make(map[string]int)}
	for _, ev := range events {
		s.Total++
		s.ByComplexity[ev.Complexity]++
		switch ev.Verdict {
		case "DOWNSHIFT":
			s.Downshifted++
		case "UPSHIFT":
			s.Upshifted++
		case "OK":
			s.OK++
		default:
			s.Unknown++
		}
		// Synthetic per-event cost estimate using the savings fraction.
		// baseline = routed / (1 - savings)
		if ev.EstimatedSavings > 0 && ev.EstimatedSavings < 1 {
			routedFraction := 1 - ev.EstimatedSavings
			// We don't have raw token costs here, so use the savings ratio
			// to back-calculate baseline from a normalised unit cost of 1.
			s.EstimatedBaselineCost += 1.0
			s.EstimatedRoutedCost += routedFraction
		} else {
			s.EstimatedBaselineCost += 1.0
			s.EstimatedRoutedCost += 1.0
		}
	}
	return s
}

// PrintStats writes a human-readable stats summary to w.
func PrintStats(events []Event, days int, w io.Writer) {
	// Filter by time window.
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	var filtered []Event
	for _, ev := range events {
		t, err := time.Parse(time.RFC3339, ev.Timestamp)
		if err != nil || t.After(cutoff) {
			filtered = append(filtered, ev)
		}
	}

	s := Aggregate(filtered)
	saved, savedFrac := s.Saved()

	label := fmt.Sprintf("Last %d days", days)
	if days <= 0 {
		label = "All time"
		filtered = events
		s = Aggregate(filtered)
		saved, savedFrac = s.Saved()
	}

	fmt.Fprintf(w, "%s\n", label)
	fmt.Fprintf(w, "─────────────────────────────────────\n")
	fmt.Fprintf(w, "Subagent decisions    %8d\n", s.Total)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "  Downshifted         %8d  %5.1f%%\n", s.Downshifted, pct(s.Downshifted, s.Total))
	fmt.Fprintf(w, "  Upshifted           %8d  %5.1f%%\n", s.Upshifted, pct(s.Upshifted, s.Total))
	fmt.Fprintf(w, "  Unchanged (OK)      %8d  %5.1f%%\n", s.OK, pct(s.OK, s.Total))
	fmt.Fprintf(w, "  Unknown             %8d  %5.1f%%\n", s.Unknown, pct(s.Unknown, s.Total))
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "By complexity\n")
	for _, cls := range []string{"TRIVIAL", "SIMPLE", "MEDIUM", "COMPLEX"} {
		n := s.ByComplexity[cls]
		fmt.Fprintf(w, "  %-8s            %8d  %5.1f%%\n", cls, n, pct(n, s.Total))
	}
	fmt.Fprintf(w, "\n")
	if s.Total > 0 && s.EstimatedBaselineCost > 0 {
		fmt.Fprintf(w, "Estimated baseline    %8.0f units\n", s.EstimatedBaselineCost)
		fmt.Fprintf(w, "Estimated routed      %8.0f units\n", s.EstimatedRoutedCost)
		fmt.Fprintf(w, "Estimated savings     %8.0f units  (%4.1f%%)\n", saved, savedFrac*100)
		fmt.Fprintf(w, "\n")
		fmt.Fprintf(w, "Note: 'units' = normalised cost per event (1 unit = baseline cost).\n")
		fmt.Fprintf(w, "For dollar figures, run:  downshift stats --cost-per-unit=<USD>\n")
	}
	fmt.Fprintf(w, "─────────────────────────────────────\n")
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
