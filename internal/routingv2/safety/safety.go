// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package safety evaluates deterministic safety rules that impose minimum tier floors.
// These rules run independently of the statistical classifier and act as a hard floor:
// the classifier may suggest a higher tier, but never lower than the safety constraint.
//
// Safety rules NEVER choose a model ID — they only constrain tier selection.
package safety

import (
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// Evaluator checks safety rules against features.
type Evaluator struct {
	rules []Rule
}

// NewEvaluator creates an evaluator with the default safety rules.
func NewEvaluator() *Evaluator {
	return &Evaluator{rules: defaultRules}
}

// NewEvaluatorWithRules creates an evaluator with custom rules (for testing).
func NewEvaluatorWithRules(rules []Rule) *Evaluator {
	return &Evaluator{rules: rules}
}

// Evaluate checks all rules and returns the highest constraint that applies.
// If no rules match, returns NoConstraint (MinTier = TierSmall).
func (e *Evaluator) Evaluate(fv domain.FeatureVector) domain.SafetyConstraint {
	var result domain.SafetyConstraint
	result.MinTier = core.TierSmall

	for _, rule := range e.rules {
		if rule.Check(fv) && rule.MinTier > result.MinTier {
			result.MinTier = rule.MinTier
			result.Reason = rule.Reason
			result.Trigger = rule.Name
		}
	}

	return result
}

// EvaluateAll returns all matching rules (for debugging/audit).
func (e *Evaluator) EvaluateAll(fv domain.FeatureVector) []Rule {
	var matched []Rule
	for _, rule := range e.rules {
		if rule.Check(fv) {
			matched = append(matched, rule)
		}
	}
	return matched
}
