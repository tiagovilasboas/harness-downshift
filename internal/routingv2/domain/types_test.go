// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package domain

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

func TestFeatureVector_AsSlice(t *testing.T) {
	fv := FeatureVector{
		Mechanical:   0.1,
		Coding:       0.2,
		Debugging:    0.3,
		Refactoring:  0.4,
		Architecture: 0.5,
		Migration:    0.6,
		Security:     0.7,
		Concurrency:  0.8,
		Planning:     0.9,
		ToolUse:      0.10,
		Ambiguity:    0.11,
		CrossModule:  0.12,
		ContextSize:  0.13,
	}

	slice := fv.AsSlice()

	if len(slice) != NumFeatures {
		t.Errorf("expected %d features, got %d", NumFeatures, len(slice))
	}

	if slice[0] != 0.1 {
		t.Errorf("expected mechanical=0.1, got %f", slice[0])
	}
	if slice[6] != 0.7 {
		t.Errorf("expected security=0.7, got %f", slice[6])
	}
}

func TestFeatureNames_Length(t *testing.T) {
	names := FeatureNames()
	if len(names) != NumFeatures {
		t.Errorf("expected %d names, got %d", NumFeatures, len(names))
	}
}

func TestTierProbabilities_MaxTier(t *testing.T) {
	tests := []struct {
		name string
		prob TierProbabilities
		want core.Tier
	}{
		{"frontier highest", TierProbabilities{0.1, 0.2, 0.7}, core.TierFrontier},
		{"mid highest", TierProbabilities{0.2, 0.6, 0.2}, core.TierMid},
		{"small highest", TierProbabilities{0.8, 0.1, 0.1}, core.TierSmall},
		{"tie frontier-mid", TierProbabilities{0.2, 0.4, 0.4}, core.TierFrontier},
		{"all equal", TierProbabilities{0.33, 0.33, 0.34}, core.TierFrontier},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.prob.MaxTier()
			if got != tt.want {
				t.Errorf("MaxTier() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTierProbabilities_Confidence(t *testing.T) {
	tests := []struct {
		name string
		prob TierProbabilities
		want float64
	}{
		{"high confidence", TierProbabilities{0.1, 0.1, 0.8}, 0.7},
		{"low confidence", TierProbabilities{0.33, 0.33, 0.34}, 0.01},
		{"medium confidence", TierProbabilities{0.1, 0.3, 0.6}, 0.3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.prob.Confidence()
			if abs(got-tt.want) > 0.01 {
				t.Errorf("Confidence() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestNoConstraint(t *testing.T) {
	c := NoConstraint()
	if c.MinTier != core.TierSmall {
		t.Errorf("NoConstraint MinTier = %v, want TierSmall", c.MinTier)
	}
	if c.Reason != "" {
		t.Errorf("NoConstraint Reason should be empty")
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
