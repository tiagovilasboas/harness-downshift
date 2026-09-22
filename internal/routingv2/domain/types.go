// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package domain defines the core types for capability-based routing v2.
// It has no dependencies on other internal packages, allowing safe import
// from anywhere in the routing pipeline.
package domain

import "github.com/tiagovilasboas/harness-downshift/internal/core"

// FeatureVector holds the 13 normalized signals extracted from a prompt.
// All values are in [0, 1]. Zero means the signal is absent; 1 means
// maximum confidence that the signal is present.
type FeatureVector struct {
	Mechanical  float64 `json:"mechanical"`   // rename, format, lint
	Coding      float64 `json:"coding"`       // produce code
	Debugging   float64 `json:"debugging"`    // investigate bugs
	Refactoring float64 `json:"refactoring"`  // restructure code
	Architecture float64 `json:"architecture"` // system design
	Migration   float64 `json:"migration"`    // data/schema migration
	Security    float64 `json:"security"`     // auth, crypto, access
	Concurrency float64 `json:"concurrency"`  // races, deadlocks
	Planning    float64 `json:"planning"`     // multi-step decomposition
	ToolUse     float64 `json:"tool_use"`     // external tools
	Ambiguity   float64 `json:"ambiguity"`    // vague requirements
	CrossModule float64 `json:"cross_module"` // multiple modules
	ContextSize float64 `json:"context_size"` // estimated context needs
}

// AsSlice returns the feature vector as a slice of float64 in canonical order.
// Used by classifiers that expect a numeric vector.
func (f FeatureVector) AsSlice() []float64 {
	return []float64{
		f.Mechanical,
		f.Coding,
		f.Debugging,
		f.Refactoring,
		f.Architecture,
		f.Migration,
		f.Security,
		f.Concurrency,
		f.Planning,
		f.ToolUse,
		f.Ambiguity,
		f.CrossModule,
		f.ContextSize,
	}
}

// FeatureNames returns the canonical names of features in slice order.
func FeatureNames() []string {
	return []string{
		"mechanical",
		"coding",
		"debugging",
		"refactoring",
		"architecture",
		"migration",
		"security",
		"concurrency",
		"planning",
		"tool_use",
		"ambiguity",
		"cross_module",
		"context_size",
	}
}

// NumFeatures is the count of features in the vector.
const NumFeatures = 13

// CapabilityReq describes the minimum capabilities required for a task.
// Values in [0, 1] where 0 means not required and 1 means maximum requirement.
type CapabilityReq struct {
	Coding      float64 `json:"coding"`
	Reasoning   float64 `json:"reasoning"`
	Planning    float64 `json:"planning"`
	Debugging   float64 `json:"debugging"`
	Security    float64 `json:"security"`
	ToolUse     float64 `json:"tool_use"`
	LongContext float64 `json:"long_context"`
}

// SafetyConstraint represents the result of deterministic safety evaluation.
// It imposes a minimum tier floor — the classifier may suggest higher, never lower.
type SafetyConstraint struct {
	MinTier core.Tier // minimum acceptable tier
	Reason  string    // human-readable explanation
	Trigger string    // which rule triggered (for audit)
}

// NoConstraint returns a SafetyConstraint with no floor (MinTier = TierSmall).
func NoConstraint() SafetyConstraint {
	return SafetyConstraint{
		MinTier: core.TierSmall,
		Reason:  "",
		Trigger: "",
	}
}

// TierProbabilities holds the classifier's probability distribution over tiers.
type TierProbabilities struct {
	Small    float64 `json:"small"`
	Mid      float64 `json:"mid"`
	Frontier float64 `json:"frontier"`
}

// MaxTier returns the tier with highest probability.
func (p TierProbabilities) MaxTier() core.Tier {
	if p.Frontier >= p.Mid && p.Frontier >= p.Small {
		return core.TierFrontier
	}
	if p.Mid >= p.Small {
		return core.TierMid
	}
	return core.TierSmall
}

// Confidence returns the gap between the highest and second-highest probability.
// Higher confidence means the classifier is more certain.
func (p TierProbabilities) Confidence() float64 {
	probs := []float64{p.Small, p.Mid, p.Frontier}

	// Find max and second max
	var max, second float64
	for _, prob := range probs {
		if prob > max {
			second = max
			max = prob
		} else if prob > second {
			second = prob
		}
	}

	return max - second
}

// RoutingInput is the input to the v2 router.
type RoutingInput struct {
	Prompt         string        // the task prompt to classify
	Harness        string        // target harness (claude-code, codex, cursor, etc.)
	CurrentModelID string        // model currently selected (may be empty)
	Resolver       core.Resolver // catalog resolver for model lookup
}

// RoutingDecision is the complete output of the v2 router.
type RoutingDecision struct {
	Tier          core.Tier         // selected tier
	Effort        core.Effort       // recommended effort level
	Model         core.Model        // selected model
	CurrentModel  core.Model        // model before routing (for savings calc)
	Verdict       core.Verdict      // whether to downshift, keep, or escalate
	Savings       float64           // cost savings ratio [0, 1)
	Confidence    float64           // classifier confidence
	Features      FeatureVector     // extracted features (for debug/telemetry)
	Safety        SafetyConstraint  // applied safety constraint
	Probabilities TierProbabilities // raw classifier output
}
