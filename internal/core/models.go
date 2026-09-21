// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package core is the harness-agnostic brain of harness-downshift.
// It classifies a task by complexity and maps it to the cheapest model tier
// that can handle it. Adapters translate this decision into each harness's
// own mechanism (Claude Code hook, Cursor frontmatter, etc).
package core

// Tier is a capability/cost bucket. Lower tiers are cheaper; higher tiers are
// stronger. The whole point of downshift is to run each subagent on the
// lowest tier that still does the job.
type Tier int

const (
	// TierSmall — cheap, fast models for mechanical work.
	TierSmall Tier = iota
	// TierMid — balanced models for everyday implementation.
	TierMid
	// TierFrontier — the strongest models, for genuinely hard work.
	TierFrontier
)

// String returns the human-readable tier name.
func (t Tier) String() string {
	switch t {
	case TierSmall:
		return "small"
	case TierMid:
		return "mid"
	case TierFrontier:
		return "frontier"
	default:
		return "unknown"
	}
}

// Model is a concrete model offered by a harness, with its tier and rough cost.
// Model IDs and costs live in the catalog package (internal/catalog/catalog.json),
// not in Go source. This struct is the value type shared between core and catalog.
type Model struct {
	ID      string  // canonical model id used by the harness
	Tier    Tier    // capability/cost bucket
	InputM  float64 // USD per 1M input tokens (approximate)
	OutputM float64 // USD per 1M output tokens (approximate)
	Harness string  // which harness this model id belongs to
}

// Effort is the reasoning/compute intensity for a task. It is harness-agnostic:
// each adapter translates this to the native scale its harness accepts
// (e.g. Codex "low/medium/high", Grok "low/medium/high").
type Effort int

const (
	// EffortLow — minimal reasoning; for mechanical, deterministic work.
	EffortLow Effort = iota
	// EffortMid — standard reasoning; for routine implementation tasks.
	EffortMid
	// EffortHigh — deep reasoning; for architecture, debugging, migrations.
	EffortHigh
)

// String returns the effort level as the canonical lowercase string.
// Adapters use this to translate to their harness-native scale.
func (e Effort) String() string {
	switch e {
	case EffortLow:
		return "low"
	case EffortMid:
		return "medium"
	case EffortHigh:
		return "high"
	default:
		return "medium"
	}
}

// EffortFor returns the recommended effort level for a tier. Cheaper tiers get
// lower effort so a trivial subagent is cheap on both model and reasoning axes.
func EffortFor(t Tier) Effort {
	switch t {
	case TierSmall:
		return EffortLow
	case TierMid:
		return EffortMid
	case TierFrontier:
		return EffortHigh
	default:
		return EffortMid
	}
}
