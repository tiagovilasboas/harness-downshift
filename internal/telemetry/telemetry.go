// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package telemetry records routing decisions to a local JSONL file and
// renders aggregated stats. All data stays on the user's machine — nothing
// is sent to any external service.
//
// Privacy note: prompt contents are never stored. Each event contains only
// routing metadata (harness, complexity class, model IDs, verdict, and a
// normalised savings fraction). This makes the event log safe for corporate
// environments where task prompts may contain sensitive information.
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
// Prompt text is intentionally excluded — only routing metadata is stored.
type Event struct {
	Timestamp        string  `json:"timestamp"`         // RFC3339
	Harness          string  `json:"harness"`           // e.g. "claude-code"
	Complexity       string  `json:"complexity"`        // TRIVIAL|SIMPLE|MEDIUM|COMPLEX
	FromModel        string  `json:"from"`              // model that would have run
	ToModel          string  `json:"to"`                // model that will run
	Verdict          string  `json:"verdict"`           // DOWNSHIFT|UPSHIFT|OK|UNKNOWN
	EstimatedSavings float64 `json:"estimated_savings"` // normalised fraction 0–1
}

// FromDecision builds an Event from a core.Decision.
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

	// NormBaseline and NormRouted are normalised cost units (1 unit = 1 baseline event).
	// They do not represent real dollars. Multiply by CostPerUnit to convert.
	NormBaseline float64
	NormRouted   float64

	// ByComplexity counts decisions per complexity class.
	ByComplexity map[string]int
}

// NormSaved returns normalised savings: absolute units saved and fraction.
func (s Stats) NormSaved() (float64, float64) {
	saved := s.NormBaseline - s.NormRouted
	if s.NormBaseline <= 0 {
		return 0, 0
	}
	return saved, saved / s.NormBaseline
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
			continue // skip malformed lines — never block on corrupted log
		}
		events = append(events, ev)
	}
	return events, sc.Err()
}

// FilterByDays returns events within the last n days.
// When n <= 0, all events are returned.
func FilterByDays(events []Event, days int) []Event {
	if days <= 0 {
		return events
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	var out []Event
	for _, ev := range events {
		t, err := time.Parse(time.RFC3339, ev.Timestamp)
		if err != nil || t.After(cutoff) {
			out = append(out, ev)
		}
	}
	return out
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
		// Normalised cost: baseline = 1.0 per event; routed = 1 - savings.
		// This is a dimensionless proxy. Multiply by --cost-per-unit to get dollars.
		s.NormBaseline += 1.0
		if ev.EstimatedSavings > 0 && ev.EstimatedSavings < 1 {
			s.NormRouted += 1 - ev.EstimatedSavings
		} else {
			s.NormRouted += 1.0
		}
	}
	return s
}

// StatsOptions controls PrintStats output.
type StatsOptions struct {
	Days        int     // time window; <= 0 means all time
	CostPerUnit float64 // USD per normalised unit; 0 means show units only
}

// PrintStats writes a human-readable stats summary to w.
func PrintStats(events []Event, opts StatsOptions, w io.Writer) {
	filtered := FilterByDays(events, opts.Days)
	s := Aggregate(filtered)
	normSaved, normFrac := s.NormSaved()

	label := fmt.Sprintf("Last %d days", opts.Days)
	if opts.Days <= 0 {
		label = "All time"
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

	if s.Total > 0 {
		if opts.CostPerUnit > 0 {
			// Dollar mode: multiply normalised units by the user-supplied rate.
			baseline := s.NormBaseline * opts.CostPerUnit
			routed := s.NormRouted * opts.CostPerUnit
			saved := normSaved * opts.CostPerUnit
			fmt.Fprintf(w, "Estimated baseline    $%10.2f\n", baseline)
			fmt.Fprintf(w, "Estimated routed      $%10.2f\n", routed)
			fmt.Fprintf(w, "Estimated savings     $%10.2f  (%4.1f%%)\n", saved, normFrac*100)
			fmt.Fprintf(w, "\n")
			fmt.Fprintf(w, "Rate: $%.4f / unit  (--cost-per-unit=%.4f)\n", opts.CostPerUnit, opts.CostPerUnit)
		} else {
			// Unit mode: dimensionless proxy, no dollar claim.
			fmt.Fprintf(w, "Normalised baseline   %8.0f units\n", s.NormBaseline)
			fmt.Fprintf(w, "Normalised routed     %8.0f units\n", s.NormRouted)
			fmt.Fprintf(w, "Normalised savings    %8.0f units  (%4.1f%%)\n", normSaved, normFrac*100)
			fmt.Fprintf(w, "\n")
			fmt.Fprintf(w, "Note: 1 unit = cost of one unrouted event. Not real dollars.\n")
			fmt.Fprintf(w, "For dollar figures: downshift stats --cost-per-unit=<USD-per-unit>\n")
		}
	}
	fmt.Fprintf(w, "─────────────────────────────────────\n")
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
