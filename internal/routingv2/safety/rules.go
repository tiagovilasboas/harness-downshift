// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package safety

import (
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// Rule represents a deterministic safety rule that may impose a minimum tier.
// Rules are evaluated in order; the highest MinTier wins.
type Rule struct {
	Name    string
	Check   func(fv domain.FeatureVector) bool
	MinTier core.Tier
	Reason  string
}

// defaultRules defines the deterministic safety floor rules.
// These are evaluated outside the statistical classifier to ensure
// high-risk tasks never route to insufficient tiers.
//
// Safety rules NEVER choose a model ID — only tier constraints.
var defaultRules = []Rule{
	{
		Name:    "security_high",
		Check:   func(fv domain.FeatureVector) bool { return fv.Security > 0.7 },
		MinTier: core.TierFrontier,
		Reason:  "auth/crypto tasks require highest capability",
	},
	{
		Name:    "migration_data",
		Check:   func(fv domain.FeatureVector) bool { return fv.Migration > 0.6 },
		MinTier: core.TierFrontier,
		Reason:  "data migrations risk data loss",
	},
	{
		Name:    "concurrency_complex",
		Check:   func(fv domain.FeatureVector) bool { return fv.Concurrency > 0.6 },
		MinTier: core.TierFrontier,
		Reason:  "race conditions require deep reasoning",
	},
	{
		Name:    "architecture_cross_module",
		Check:   func(fv domain.FeatureVector) bool { return fv.Architecture > 0.7 && fv.CrossModule > 0.5 },
		MinTier: core.TierFrontier,
		Reason:  "cross-module architecture has high blast radius",
	},
	{
		Name:    "security_moderate",
		Check:   func(fv domain.FeatureVector) bool { return fv.Security > 0.4 && fv.Security <= 0.7 },
		MinTier: core.TierMid,
		Reason:  "security tasks need at least mid-tier capability",
	},
	{
		Name:    "ambiguity_high",
		Check:   func(fv domain.FeatureVector) bool { return fv.Ambiguity > 0.8 },
		MinTier: core.TierMid,
		Reason:  "ambiguous requirements need clarification capability",
	},
	{
		Name:    "debugging_complex",
		Check:   func(fv domain.FeatureVector) bool { return fv.Debugging > 0.6 && fv.CrossModule > 0.4 },
		MinTier: core.TierMid,
		Reason:  "cross-module debugging needs broader context",
	},
}
