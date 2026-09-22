// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package policy

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

func TestDecide_ClassifierWins(t *testing.T) {
	probs := domain.TierProbabilities{Small: 0.1, Mid: 0.2, Frontier: 0.7}
	safety := domain.NoConstraint()

	tier := Decide(probs, safety)
	if tier != core.TierFrontier {
		t.Errorf("expected Frontier (classifier), got %v", tier)
	}
}

func TestDecide_SafetyFloor(t *testing.T) {
	// Classifier says Small, but safety says Mid
	probs := domain.TierProbabilities{Small: 0.7, Mid: 0.2, Frontier: 0.1}
	safety := domain.SafetyConstraint{MinTier: core.TierMid, Trigger: "test"}

	tier := Decide(probs, safety)
	if tier != core.TierMid {
		t.Errorf("expected Mid (safety floor), got %v", tier)
	}
}

func TestDecide_SafetyFloorFrontier(t *testing.T) {
	// Classifier says Mid, but safety says Frontier
	probs := domain.TierProbabilities{Small: 0.1, Mid: 0.7, Frontier: 0.2}
	safety := domain.SafetyConstraint{MinTier: core.TierFrontier, Trigger: "security_high"}

	tier := Decide(probs, safety)
	if tier != core.TierFrontier {
		t.Errorf("expected Frontier (safety floor), got %v", tier)
	}
}

func TestDecide_LowConfidence(t *testing.T) {
	// Classifier says Small with low confidence
	probs := domain.TierProbabilities{Small: 0.4, Mid: 0.35, Frontier: 0.25}
	safety := domain.NoConstraint()

	confidence := probs.Confidence()
	if confidence >= LowConfidenceThreshold {
		t.Fatalf("test setup wrong: confidence %f >= threshold", confidence)
	}

	tier := Decide(probs, safety)
	if tier != core.TierMid {
		t.Errorf("expected Mid (low confidence conservative), got %v", tier)
	}
}

func TestDecide_HighConfidenceSmall(t *testing.T) {
	// Classifier says Small with high confidence → trust it
	probs := domain.TierProbabilities{Small: 0.85, Mid: 0.1, Frontier: 0.05}
	safety := domain.NoConstraint()

	confidence := probs.Confidence()
	if confidence < LowConfidenceThreshold {
		t.Fatalf("test setup wrong: confidence %f < threshold", confidence)
	}

	tier := Decide(probs, safety)
	if tier != core.TierSmall {
		t.Errorf("expected Small (high confidence), got %v", tier)
	}
}

func TestDecide_SafetyOverridesLowConfidence(t *testing.T) {
	// Low confidence pushes to Mid, but safety pushes to Frontier
	probs := domain.TierProbabilities{Small: 0.4, Mid: 0.35, Frontier: 0.25}
	safety := domain.SafetyConstraint{MinTier: core.TierFrontier, Trigger: "migration"}

	tier := Decide(probs, safety)
	if tier != core.TierFrontier {
		t.Errorf("expected Frontier (safety > low_confidence), got %v", tier)
	}
}

func TestDecideWithDetails_Reasons(t *testing.T) {
	tests := []struct {
		name   string
		probs  domain.TierProbabilities
		safety domain.SafetyConstraint
		want   string
	}{
		{
			"classifier",
			domain.TierProbabilities{Small: 0.1, Mid: 0.1, Frontier: 0.8},
			domain.NoConstraint(),
			"classifier",
		},
		{
			"low_confidence",
			domain.TierProbabilities{Small: 0.4, Mid: 0.35, Frontier: 0.25},
			domain.NoConstraint(),
			"low_confidence",
		},
		{
			"safety",
			domain.TierProbabilities{Small: 0.8, Mid: 0.15, Frontier: 0.05},
			domain.SafetyConstraint{MinTier: core.TierFrontier, Trigger: "security_high"},
			"safety:security_high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reason := DecideWithDetails(tt.probs, tt.safety)
			if reason != tt.want {
				t.Errorf("reason = %q, want %q", reason, tt.want)
			}
		})
	}
}
