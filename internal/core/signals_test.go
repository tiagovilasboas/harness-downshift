// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

import (
	"fmt"
	"regexp"
	"testing"
)

// TestRawSignals_NonEmpty ensures the signal table is never accidentally cleared.
func TestRawSignals_NonEmpty(t *testing.T) {
	if len(RawSignals) == 0 {
		t.Fatal("RawSignals is empty — classifier has no signals")
	}
}

// TestRawSignals_AllPatternsCompile verifies every regex in the table is valid.
// A bad pattern panics at startup (compileSignals calls MustCompile); this test
// catches it at test time with a clear failure message.
func TestRawSignals_AllPatternsCompile(t *testing.T) {
	for i, s := range RawSignals {
		pat := `(?i)` + s.Pattern
		if _, err := regexp.Compile(pat); err != nil {
			t.Errorf("signal[%d] pattern %q does not compile: %v", i, s.Pattern, err)
		}
	}
}

// TestRawSignals_ValidClasses ensures every signal votes for a known complexity.
func TestRawSignals_ValidClasses(t *testing.T) {
	valid := map[Complexity]bool{Trivial: true, Simple: true, Medium: true, Complex: true}
	for i, s := range RawSignals {
		if !valid[s.Class] {
			t.Errorf("signal[%d] pattern %q has unknown class %v", i, s.Pattern, s.Class)
		}
	}
}

// TestRawSignals_WeightsPositive ensures no signal has a zero or negative weight.
func TestRawSignals_WeightsPositive(t *testing.T) {
	for i, s := range RawSignals {
		if s.Weight <= 0 {
			t.Errorf("signal[%d] pattern %q has non-positive weight %d", i, s.Pattern, s.Weight)
		}
	}
}

// TestRawSignals_NoDuplicatePatterns detects accidentally duplicated patterns.
func TestRawSignals_NoDuplicatePatterns(t *testing.T) {
	seen := make(map[string]int) // pattern → first index
	for i, s := range RawSignals {
		if prev, exists := seen[s.Pattern]; exists {
			t.Errorf("signal[%d] duplicates signal[%d]: pattern %q", i, prev, s.Pattern)
		}
		seen[s.Pattern] = i
	}
}

// TestRawSignals_AllClassesRepresented ensures every complexity class has at
// least one signal — a missing class would leave tasks unclassifiable.
func TestRawSignals_AllClassesRepresented(t *testing.T) {
	counts := map[Complexity]int{}
	for _, s := range RawSignals {
		counts[s.Class]++
	}
	for _, c := range []Complexity{Trivial, Simple, Medium, Complex} {
		if counts[c] == 0 {
			t.Errorf("no signals for complexity class %s", c)
		}
	}
}

// TestRawSignals_ConsistentWithCompiledSignals verifies that the compiled
// signals slice (used by Classify) has the same length as RawSignals. If they
// diverge, compileSignals is not sourcing from RawSignals correctly.
func TestRawSignals_ConsistentWithCompiledSignals(t *testing.T) {
	if len(signals) != len(RawSignals) {
		t.Errorf("compiled signals length %d != RawSignals length %d",
			len(signals), len(RawSignals))
	}
}

// TestRawSignals_CoverageByClass prints a summary — useful when tuning.
func TestRawSignals_CoverageByClass(t *testing.T) {
	counts := map[Complexity]int{}
	for _, s := range RawSignals {
		counts[s.Class]++
	}
	t.Logf("signal distribution: TRIVIAL=%d SIMPLE=%d MEDIUM=%d COMPLEX=%d total=%d",
		counts[Trivial], counts[Simple], counts[Medium], counts[Complex], len(RawSignals))
	_ = fmt.Sprintf // keep import
}
