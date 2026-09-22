// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package classifier

import (
	"math"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

func TestSoftmax_SumsToOne(t *testing.T) {
	probs := softmax(1.0, 2.0, 3.0)
	sum := probs[0] + probs[1] + probs[2]
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("softmax should sum to 1, got %f", sum)
	}
}

func TestSoftmax_Ordering(t *testing.T) {
	probs := softmax(1.0, 2.0, 3.0)
	// Higher score → higher probability
	if probs[2] <= probs[1] || probs[1] <= probs[0] {
		t.Errorf("softmax should preserve ordering: %v", probs)
	}
}

func TestSoftmax_NumericalStability(t *testing.T) {
	// Large values that would overflow without log-sum-exp trick
	probs := softmax(1000.0, 1001.0, 1002.0)
	sum := probs[0] + probs[1] + probs[2]
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("softmax should handle large values, got sum=%f", sum)
	}
}

func TestClassifier_MechanicalTask(t *testing.T) {
	c := NewSoftmaxClassifier(DefaultWeights())

	fv := domain.FeatureVector{
		Mechanical: 0.9,
		Coding:     0.1,
	}

	probs, err := c.Classify(fv)
	if err != nil {
		t.Fatal(err)
	}

	// Mechanical tasks should lean toward Small
	if probs.MaxTier() != core.TierSmall {
		t.Errorf("mechanical task should prefer Small, got %v (probs: %+v)",
			probs.MaxTier(), probs)
	}
}

func TestClassifier_SecurityTask(t *testing.T) {
	c := NewSoftmaxClassifier(DefaultWeights())

	fv := domain.FeatureVector{
		Security:    0.9,
		Coding:      0.5,
		CrossModule: 0.3,
	}

	probs, err := c.Classify(fv)
	if err != nil {
		t.Fatal(err)
	}

	// Security tasks should lean toward Frontier
	if probs.MaxTier() != core.TierFrontier {
		t.Errorf("security task should prefer Frontier, got %v (probs: %+v)",
			probs.MaxTier(), probs)
	}
}

func TestClassifier_CodingTask(t *testing.T) {
	c := NewSoftmaxClassifier(DefaultWeights())

	fv := domain.FeatureVector{
		Coding:      0.7,
		Refactoring: 0.4,
	}

	probs, err := c.Classify(fv)
	if err != nil {
		t.Fatal(err)
	}

	// Standard coding should lean toward Mid
	if probs.MaxTier() != core.TierMid {
		t.Errorf("coding task should prefer Mid, got %v (probs: %+v)",
			probs.MaxTier(), probs)
	}
}

func TestClassifier_AllProbsValid(t *testing.T) {
	c := NewSoftmaxClassifier(DefaultWeights())

	testCases := []domain.FeatureVector{
		{},                                  // zero features
		{Mechanical: 1.0},                   // single high
		{Security: 1.0, Migration: 1.0},     // multiple high
		{Coding: 0.5, Debugging: 0.5},       // balanced
	}

	for i, fv := range testCases {
		probs, err := c.Classify(fv)
		if err != nil {
			t.Errorf("case %d: unexpected error: %v", i, err)
			continue
		}

		// All probabilities should be in [0, 1]
		for _, p := range []float64{probs.Small, probs.Mid, probs.Frontier} {
			if p < 0 || p > 1 {
				t.Errorf("case %d: probability out of range: %f", i, p)
			}
		}

		// Sum should be 1
		sum := probs.Small + probs.Mid + probs.Frontier
		if math.Abs(sum-1.0) > 1e-9 {
			t.Errorf("case %d: probabilities should sum to 1, got %f", i, sum)
		}
	}
}

func TestLoadEmbeddedWeights(t *testing.T) {
	w, err := LoadEmbeddedWeights()
	if err != nil {
		t.Fatalf("failed to load embedded weights: %v", err)
	}

	if w.Version == "" {
		t.Error("weights should have version")
	}

	// Check that weights are populated
	if w.Small.Bias == 0 && w.Mid.Bias == 0 && w.Frontier.Bias == 0 {
		// At least one should be non-zero in default weights
		hasNonZero := false
		for _, v := range w.Small.W {
			if v != 0 {
				hasNonZero = true
				break
			}
		}
		if !hasNonZero {
			t.Error("weights appear to be all zeros")
		}
	}
}

func TestRiskWeight(t *testing.T) {
	tests := []struct {
		actual    core.Tier
		predicted core.Tier
		minWeight float64
	}{
		// Catastrophic
		{core.TierFrontier, core.TierSmall, 9.0},
		// Expensive
		{core.TierFrontier, core.TierMid, 4.0},
		// Moderate
		{core.TierMid, core.TierSmall, 1.5},
		// Cheap (over-routing)
		{core.TierSmall, core.TierFrontier, 0.4},
		// Correct
		{core.TierMid, core.TierMid, 0.9},
	}

	for _, tt := range tests {
		w := RiskWeight(tt.actual, tt.predicted)
		if w < tt.minWeight {
			t.Errorf("RiskWeight(%v, %v) = %f, expected >= %f",
				tt.actual, tt.predicted, w, tt.minWeight)
		}
	}
}

func TestIsUnsafeDowngrade(t *testing.T) {
	tests := []struct {
		actual    core.Tier
		predicted core.Tier
		unsafe    bool
	}{
		{core.TierFrontier, core.TierSmall, true},
		{core.TierFrontier, core.TierMid, true},
		{core.TierMid, core.TierSmall, true},
		{core.TierSmall, core.TierMid, false},
		{core.TierSmall, core.TierFrontier, false},
		{core.TierMid, core.TierFrontier, false},
		{core.TierMid, core.TierMid, false},
	}

	for _, tt := range tests {
		got := IsUnsafeDowngrade(tt.actual, tt.predicted)
		if got != tt.unsafe {
			t.Errorf("IsUnsafeDowngrade(%v, %v) = %v, want %v",
				tt.actual, tt.predicted, got, tt.unsafe)
		}
	}
}
