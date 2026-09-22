// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package policy decides the target tier based on classifier output,
// confidence level, and safety constraints. It implements risk-aware
// routing: low confidence → conservative choice.
package policy

import (
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// LowConfidenceThreshold is the confidence level below which we route conservatively.
const LowConfidenceThreshold = 0.4

// Decide returns the target tier considering classifier output, confidence, and safety.
// The returned tier is always >= safety.MinTier (safety is a hard floor).
// When confidence is low, we prefer a higher tier to be safe.
func Decide(probs domain.TierProbabilities, safety domain.SafetyConstraint) core.Tier {
	classifierTier := probs.MaxTier()
	confidence := probs.Confidence()

	// Start with the classifier's suggestion
	tier := classifierTier

	// Low confidence → be conservative (at least Mid)
	if confidence < LowConfidenceThreshold && tier < core.TierMid {
		tier = core.TierMid
	}

	// Safety is a hard floor
	if safety.MinTier > tier {
		tier = safety.MinTier
	}

	return tier
}

// DecideWithDetails returns the tier and explains the decision.
func DecideWithDetails(probs domain.TierProbabilities, safety domain.SafetyConstraint) (core.Tier, string) {
	classifierTier := probs.MaxTier()
	confidence := probs.Confidence()
	tier := classifierTier
	reason := "classifier"

	// Low confidence → be conservative
	if confidence < LowConfidenceThreshold && tier < core.TierMid {
		tier = core.TierMid
		reason = "low_confidence"
	}

	// Safety floor
	if safety.MinTier > tier {
		tier = safety.MinTier
		reason = "safety:" + safety.Trigger
	}

	return tier, reason
}
