// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

// Resolver maps a (harness, tier) pair to the concrete model the harness
// should use, and computes the savings ratio between two models.
//
// The interface lives in core so that policy.go can accept it without
// importing the catalog package — keeping the dependency direction clean:
// catalog → core, never core → catalog.
//
// The default implementation is in internal/catalog. The catalog package
// loads model data from an embedded JSON file (with an optional user
// override at ~/.harness-downshift/catalog.json) and implements Resolver.
type Resolver interface {
	// ModelFor returns the Model to use for the given harness and tier.
	// If the harness is unknown it must return a safe fallback (not zero).
	ModelFor(harness string, tier Tier) Model

	// LookupByID returns the Model with the given ID in the given harness
	// catalog, and whether it was found.
	LookupByID(harness, modelID string) (Model, bool)

	// SavingsRatio returns how much cheaper `to` is versus `from` as a
	// fraction in [0, 1). Returns 0 when `to` is not cheaper than `from`.
	SavingsRatio(from, to Model) float64

	// EffortFor translates a core.Effort level to the harness-native string
	// for the given model ID (e.g. Codex "low"/"medium"/"high", Claude
	// thinking budget "0"/"8000"/"16000"). Falls back to effort.String()
	// when no mapping is found — adapters can always call this safely.
	EffortFor(harness, modelID string, effort Effort) string
}
