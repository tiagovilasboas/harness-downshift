// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package benchmark evaluates the classifier against a hand-labelled task
// dataset and reports a confusion matrix, accuracy, and the false-downshift
// rate — the fraction of COMPLEX tasks that were mis-routed to a cheaper tier.
//
// Dataset format (JSON):
//
//	[
//	  { "prompt": "rename the userId variable", "label": "TRIVIAL" },
//	  { "prompt": "rearchitect the auth module", "label": "COMPLEX" }
//	]
//
// Labels are case-insensitive: TRIVIAL, SIMPLE, MEDIUM, COMPLEX.
package benchmark

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// Task is one entry in the benchmark dataset.
type Task struct {
	Prompt string `json:"prompt"`
	Label  string `json:"label"` // expected complexity: TRIVIAL|SIMPLE|MEDIUM|COMPLEX
}

// Result holds the outcome for one task.
type Result struct {
	Task      Task
	Predicted core.Complexity
	Correct   bool
}

// complexityFromString parses a label string into a Complexity.
// Returns Medium and false when the label is unknown.
func complexityFromString(s string) (core.Complexity, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRIVIAL":
		return core.Trivial, true
	case "SIMPLE":
		return core.Simple, true
	case "MEDIUM":
		return core.Medium, true
	case "COMPLEX":
		return core.Complex, true
	default:
		return core.Medium, false
	}
}

// LoadDataset reads a JSON task dataset from path.
func LoadDataset(path string) ([]Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading dataset: %w", err)
	}
	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("parsing dataset: %w", err)
	}
	return tasks, nil
}

// Run classifies every task and returns the per-task results.
// Tasks with unrecognised labels are skipped (logged to w).
func Run(tasks []Task, w io.Writer) []Result {
	var results []Result
	skipped := 0
	for _, t := range tasks {
		expected, ok := complexityFromString(t.Label)
		if !ok {
			fmt.Fprintf(w, "skip: unknown label %q for prompt %q\n", t.Label, t.Prompt)
			skipped++
			continue
		}
		cls := core.Classify(t.Prompt)
		results = append(results, Result{
			Task:      t,
			Predicted: cls.Complexity,
			Correct:   cls.Complexity == expected,
		})
	}
	if skipped > 0 {
		fmt.Fprintf(w, "skipped %d tasks with unrecognised labels\n\n", skipped)
	}
	return results
}

// Matrix is a 4×4 confusion matrix indexed by [actual][predicted].
// Row = actual label, Column = predicted label.
// Order: Trivial=0, Simple=1, Medium=2, Complex=3.
type Matrix [4][4]int

// Build computes the confusion matrix from results.
func Build(results []Result) Matrix {
	var m Matrix
	for _, r := range results {
		expected, ok := complexityFromString(r.Task.Label)
		if !ok {
			continue
		}
		m[expected][r.Predicted]++
	}
	return m
}

// Accuracy returns the fraction of correct predictions.
func Accuracy(results []Result) float64 {
	if len(results) == 0 {
		return 0
	}
	correct := 0
	for _, r := range results {
		if r.Correct {
			correct++
		}
	}
	return float64(correct) / float64(len(results))
}

// FalseDownshiftRate returns the fraction of COMPLEX tasks that were routed
// to a cheaper tier (TRIVIAL, SIMPLE, or MEDIUM). This is the safety-critical
// metric: routing a COMPLEX task to a weak model can produce wrong output.
func FalseDownshiftRate(results []Result) (rate float64, count int, total int) {
	for _, r := range results {
		expected, ok := complexityFromString(r.Task.Label)
		if !ok || expected != core.Complex {
			continue
		}
		total++
		if r.Predicted < core.Complex {
			count++
		}
	}
	if total == 0 {
		return 0, 0, 0
	}
	return float64(count) / float64(total), count, total
}

// OverRoutingRate returns the fraction of TRIVIAL tasks routed to a more
// expensive tier. Over-routing wastes money but is safe.
func OverRoutingRate(results []Result) (rate float64, count int, total int) {
	for _, r := range results {
		expected, ok := complexityFromString(r.Task.Label)
		if !ok || expected != core.Trivial {
			continue
		}
		total++
		if r.Predicted > core.Trivial {
			count++
		}
	}
	if total == 0 {
		return 0, 0, 0
	}
	return float64(count) / float64(total), count, total
}

var labels = []string{"T", "S", "M", "C"}
var fullLabels = []string{"TRIVIAL", "SIMPLE", "MEDIUM", "COMPLEX"}

// Print writes the full benchmark report to w.
func Print(results []Result, m Matrix, w io.Writer) {
	n := len(results)
	acc := Accuracy(results)
	fdRate, fdCount, fdTotal := FalseDownshiftRate(results)
	orRate, orCount, orTotal := OverRoutingRate(results)

	fmt.Fprintf(w, "Dataset: %d tasks\n\n", n)
	fmt.Fprintf(w, "Accuracy          %6.1f%%\n", acc*100)
	fmt.Fprintf(w, "Under-routing      %6.1f%%  (%d / %d COMPLEX mis-routed cheaper)\n",
		fdRate*100, fdCount, fdTotal)
	fmt.Fprintf(w, "Over-routing       %6.1f%%  (%d / %d TRIVIAL mis-routed dearer)\n",
		orRate*100, orCount, orTotal)
	fmt.Fprintf(w, "\n")

	// False downshift detail: which COMPLEX tasks went where.
	fmt.Fprintf(w, "False downshift detail (COMPLEX → cheaper tier)\n")
	for _, r := range results {
		expected, ok := complexityFromString(r.Task.Label)
		if !ok || expected != core.Complex || r.Correct {
			continue
		}
		fmt.Fprintf(w, "  COMPLEX → %-7s  %q\n", r.Predicted.String(), truncate(r.Task.Prompt, 70))
	}
	fmt.Fprintf(w, "\n")

	// Confusion matrix.
	fmt.Fprintf(w, "Confusion matrix\n\n")
	fmt.Fprintf(w, "             ")
	for _, l := range labels {
		fmt.Fprintf(w, "  %4s", l)
	}
	fmt.Fprintf(w, "   (predicted)\n")
	for row, rowLabel := range fullLabels {
		fmt.Fprintf(w, "Actual %-7s", rowLabel)
		for col := range labels {
			fmt.Fprintf(w, "  %4d", m[row][col])
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "\nT=TRIVIAL  S=SIMPLE  M=MEDIUM  C=COMPLEX\n")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
