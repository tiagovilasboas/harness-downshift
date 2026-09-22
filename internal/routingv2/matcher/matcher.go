// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package matcher selects the cheapest model that satisfies tier and capability requirements.
// It uses the catalog (via core.Resolver) but makes decisions based on tier and cost,
// never on model names or providers — keeping the router model-agnostic.
package matcher

import (
	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"
)

// Match selects the cheapest model from the resolver that satisfies the target tier.
// The resolver interface abstracts the catalog, keeping matcher decoupled from catalog implementation.
//
// Algorithm:
// 1. Get model for target tier from resolver
// 2. If explicit_only, fall back to tier default
// 3. Return the cheapest valid model
//
// This is a simplified matcher. A full implementation would iterate all models,
// filter by tier >= target, filter by capabilities >= requirements, exclude explicit_only,
// and sort by cost. For now, we trust the resolver's ModelFor to do the right thing.
func Match(harness string, tier core.Tier, resolver core.Resolver) core.Model {
	model := resolver.ModelFor(harness, tier)

	// Check if this model is marked explicit_only
	if resolver.IsExplicitOnly(harness, model.ID) {
		// Fall back: try to get next tier up, or just return what we have (fail-open)
		if tier < core.TierFrontier {
			fallback := resolver.ModelFor(harness, tier+1)
			if !resolver.IsExplicitOnly(harness, fallback.ID) {
				return fallback
			}
		}
	}

	return model
}

// MatchWithProfile is an extended matcher that also considers capability requirements.
// This is the full algorithm from the design doc, but requires profiles to be available
// in the catalog. For now, it falls back to Match when profiles aren't available.
func MatchWithProfile(harness string, tier core.Tier, req domain.CapabilityReq, profiles []domain.ModelProfile, resolver core.Resolver) core.Model {
	// If no profiles provided, fall back to simple matching
	if len(profiles) == 0 {
		return Match(harness, tier, resolver)
	}

	var best *domain.ModelProfile
	var bestCost float64 = -1

	for i := range profiles {
		p := &profiles[i]

		// Filter: must be for this harness
		if p.Harness != harness {
			continue
		}

		// Filter: must meet tier requirement
		if p.Tier < tier {
			continue
		}

		// Filter: must not be explicit_only
		if p.ExplicitOnly {
			continue
		}

		// Filter: must satisfy capability requirements
		if !p.Capabilities.Satisfies(req) {
			continue
		}

		// Select cheapest
		cost := p.TotalCost()
		if best == nil || cost < bestCost {
			best = p
			bestCost = cost
		}
	}

	if best != nil {
		return core.Model{
			ID:      best.ID,
			Tier:    best.Tier,
			InputM:  best.InputCost,
			OutputM: best.OutputCost,
			Harness: best.Harness,
		}
	}

	// Fail-open: fall back to resolver
	return Match(harness, tier, resolver)
}
