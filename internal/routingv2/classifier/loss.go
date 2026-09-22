// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package classifier

import (
	"math"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// RiskWeights defines asymmetric penalties for routing errors.
// Under-routing (sending hard tasks to weak models) is penalized
// more heavily than over-routing (wasting money on easy tasks).
var RiskWeights = map[[2]core.Tier]float64{
	// Catastrophic: frontier task to small model
	{core.TierFrontier, core.TierSmall}: 10.0,

	// Expensive: frontier task to mid model
	{core.TierFrontier, core.TierMid}: 5.0,

	// Moderate: mid task to small model
	{core.TierMid, core.TierSmall}: 2.0,

	// Cheap: over-routing wastes money but is safe
	{core.TierSmall, core.TierMid}:      0.5,
	{core.TierSmall, core.TierFrontier}: 1.0,
	{core.TierMid, core.TierFrontier}:   0.5,

	// Correct predictions
	{core.TierSmall, core.TierSmall}:       1.0,
	{core.TierMid, core.TierMid}:           1.0,
	{core.TierFrontier, core.TierFrontier}: 1.0,
}

// RiskWeight returns the penalty multiplier for a prediction error.
func RiskWeight(actual, predicted core.Tier) float64 {
	if w, ok := RiskWeights[[2]core.Tier{actual, predicted}]; ok {
		return w
	}
	return 1.0 // default
}

// CrossEntropyLoss computes -log(p) for the correct class.
// Lower is better. Used for training.
func CrossEntropyLoss(probs [3]float64, actualTier core.Tier) float64 {
	p := probs[actualTier]
	if p < 1e-10 {
		p = 1e-10 // avoid log(0)
	}
	return -math.Log(p)
}

// RiskWeightedLoss combines cross-entropy with risk penalty.
// This is the primary loss function for training.
func RiskWeightedLoss(probs [3]float64, actualTier, predictedTier core.Tier) float64 {
	ce := CrossEntropyLoss(probs, actualTier)
	rw := RiskWeight(actualTier, predictedTier)
	return ce * rw
}

// IsUnsafeDowngrade returns true if this is a dangerous under-routing.
func IsUnsafeDowngrade(actual, predicted core.Tier) bool {
	// Frontier task sent to non-frontier
	if actual == core.TierFrontier && predicted != core.TierFrontier {
		return true
	}
	// Mid task sent to small
	if actual == core.TierMid && predicted == core.TierSmall {
		return true
	}
	return false
}
