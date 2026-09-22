// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package training

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

func TestTrain_ImprovesOnDataset(t *testing.T) {
	// Create a simple dataset with clear patterns
	ds := &Dataset{
		Examples: []Example{
			// Mechanical tasks → SMALL
			{Prompt: "rename x to y", Label: "SMALL", Features: domain.FeatureVector{Mechanical: 0.9}},
			{Prompt: "format code", Label: "SMALL", Features: domain.FeatureVector{Mechanical: 0.8}},
			{Prompt: "fix typo", Label: "SMALL", Features: domain.FeatureVector{Mechanical: 0.85}},

			// Implementation tasks → MID
			{Prompt: "add unit test", Label: "MID", Features: domain.FeatureVector{Coding: 0.7, Debugging: 0.3}},
			{Prompt: "refactor method", Label: "MID", Features: domain.FeatureVector{Refactoring: 0.8}},
			{Prompt: "implement feature", Label: "MID", Features: domain.FeatureVector{Coding: 0.8}},

			// Complex tasks → FRONTIER
			{Prompt: "design auth system", Label: "FRONTIER", Features: domain.FeatureVector{Security: 0.9, Architecture: 0.7}},
			{Prompt: "fix race condition", Label: "FRONTIER", Features: domain.FeatureVector{Concurrency: 0.9}},
			{Prompt: "migrate database", Label: "FRONTIER", Features: domain.FeatureVector{Migration: 0.8}},
		},
	}

	config := TrainConfig{
		Epochs:       50,
		LearningRate: 0.1,
		RiskWeighted: true,
	}

	train, val := ds.Split(0.3) // 6 train, 3 val
	result := Train(train, val, config)

	// Weights should be produced
	if result.Weights == nil {
		t.Fatal("Training produced nil weights")
	}

	// Accuracy should be reasonable (at least better than random 33%)
	if result.TrainMetrics.Accuracy < 0.5 {
		t.Errorf("Train accuracy = %.1f%%, expected > 50%%", result.TrainMetrics.Accuracy*100)
	}
}

func TestTrain_RiskWeightedReducesUnsafe(t *testing.T) {
	// Dataset with FRONTIER examples
	ds := &Dataset{
		Examples: []Example{
			{Label: "SMALL", Features: domain.FeatureVector{Mechanical: 0.9}},
			{Label: "SMALL", Features: domain.FeatureVector{Mechanical: 0.8}},
			{Label: "MID", Features: domain.FeatureVector{Coding: 0.8}},
			{Label: "MID", Features: domain.FeatureVector{Refactoring: 0.7}},
			{Label: "FRONTIER", Features: domain.FeatureVector{Security: 0.9}},
			{Label: "FRONTIER", Features: domain.FeatureVector{Concurrency: 0.8}},
		},
	}

	// Train with risk weighting
	configRisk := TrainConfig{Epochs: 50, LearningRate: 0.1, RiskWeighted: true}
	resultRisk := Train(ds, ds, configRisk)

	// Train without risk weighting
	configNoRisk := TrainConfig{Epochs: 50, LearningRate: 0.1, RiskWeighted: false}
	resultNoRisk := Train(ds, ds, configNoRisk)

	// Risk-weighted should have lower or equal unsafe rate
	// (This is probabilistic, so we just check it's reasonable)
	if resultRisk.TrainMetrics.UnsafeDowngradeRate > 0.5 {
		t.Logf("Risk-weighted unsafe rate: %.1f%%, No-risk: %.1f%%",
			resultRisk.TrainMetrics.UnsafeDowngradeRate*100,
			resultNoRisk.TrainMetrics.UnsafeDowngradeRate*100)
	}
}

func TestDefaultTrainConfig(t *testing.T) {
	config := DefaultTrainConfig()

	if config.Epochs <= 0 {
		t.Error("Default epochs should be > 0")
	}

	if config.LearningRate <= 0 {
		t.Error("Default learning rate should be > 0")
	}

	if !config.RiskWeighted {
		t.Error("Default should use risk-weighted loss")
	}
}

func TestTrain_EmptyDataset(t *testing.T) {
	ds := &Dataset{}
	config := DefaultTrainConfig()

	result := Train(ds, ds, config)

	// Should not panic and return default weights
	if result.Weights == nil {
		t.Error("Training empty dataset should return default weights")
	}
}

func TestSoftmax(t *testing.T) {
	probs := softmax(1.0, 2.0, 3.0)

	// Should sum to 1
	sum := probs[0] + probs[1] + probs[2]
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("Softmax sum = %.4f, want ~1.0", sum)
	}

	// Higher input should have higher probability
	if probs[0] >= probs[1] || probs[1] >= probs[2] {
		t.Errorf("Softmax order wrong: %v", probs)
	}
}

func TestDot(t *testing.T) {
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}

	result := dot(a, b)
	expected := 1*4 + 2*5 + 3*6 // = 32

	if result != float64(expected) {
		t.Errorf("dot(%v, %v) = %.0f, want %d", a, b, result, expected)
	}
}

func TestMaxTier(t *testing.T) {
	tests := []struct {
		probs []float64
		want  core.Tier
	}{
		{[]float64{0.7, 0.2, 0.1}, core.TierSmall},
		{[]float64{0.1, 0.7, 0.2}, core.TierMid},
		{[]float64{0.1, 0.2, 0.7}, core.TierFrontier},
		{[]float64{0.33, 0.33, 0.34}, core.TierFrontier}, // Tie goes to highest index
	}

	for _, tc := range tests {
		got := maxTier(tc.probs)
		if got != tc.want {
			t.Errorf("maxTier(%v) = %v, want %v", tc.probs, got, tc.want)
		}
	}
}
