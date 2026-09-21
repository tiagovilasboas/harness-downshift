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
type Model struct {
	ID       string  // canonical model id used by the harness
	Tier     Tier    // capability/cost bucket
	InputM   float64 // USD per 1M input tokens (approximate)
	OutputM  float64 // USD per 1M output tokens (approximate)
	Harness  string  // which harness this model id belongs to
}

// Catalog maps each harness to its models per tier. These are the model ids
// each harness actually accepts. Prices are approximate (Sep 2026) and only
// used to show the user the savings — the routing decision is tier-based.
var Catalog = map[string]map[Tier]Model{
	"claude-code": {
		TierSmall:    {ID: "claude-haiku-4", Tier: TierSmall, InputM: 0.80, OutputM: 4.00, Harness: "claude-code"},
		TierMid:      {ID: "claude-sonnet-4-6", Tier: TierMid, InputM: 3.00, OutputM: 15.00, Harness: "claude-code"},
		TierFrontier: {ID: "claude-opus-4-8", Tier: TierFrontier, InputM: 15.00, OutputM: 75.00, Harness: "claude-code"},
	},
	"cursor": {
		TierSmall:    {ID: "claude-haiku-4", Tier: TierSmall, InputM: 0.80, OutputM: 4.00, Harness: "cursor"},
		TierMid:      {ID: "claude-sonnet-4.6", Tier: TierMid, InputM: 3.00, OutputM: 15.00, Harness: "cursor"},
		TierFrontier: {ID: "claude-opus-4.8", Tier: TierFrontier, InputM: 15.00, OutputM: 75.00, Harness: "cursor"},
	},
	// Codex model ids as accepted by the CLI / Responses API (Sep 2026).
	// gpt-5.6-luna is the lowest cost/latency reasoning model; gpt-5.6-terra
	// is the balanced tier; gpt-5.3-codex is the coding-tuned frontier model
	// (supports low/medium/high/xhigh reasoning effort). See
	// developers.openai.com/codex/models and .../api/docs/guides/reasoning.
	"codex": {
		TierSmall:    {ID: "gpt-5.6-luna", Tier: TierSmall, InputM: 0.15, OutputM: 0.60, Harness: "codex"},
		TierMid:      {ID: "gpt-5.6-terra", Tier: TierMid, InputM: 1.25, OutputM: 5.00, Harness: "codex"},
		TierFrontier: {ID: "gpt-5.3-codex", Tier: TierFrontier, InputM: 10.00, OutputM: 40.00, Harness: "codex"},
	},
}

// ModelFor returns the model a harness should use for a given tier.
// Falls back to the generic Claude tiers if the harness is unknown.
func ModelFor(harness string, tier Tier) Model {
	if models, ok := Catalog[harness]; ok {
		if m, ok := models[tier]; ok {
			return m
		}
	}
	// Fallback: generic Claude naming.
	return Catalog["claude-code"][tier]
}

// SavingsRatio returns how much cheaper `to` is versus `from`, as a fraction
// (0.80 == 80% cheaper). Uses a blended input+output cost. Returns 0 when the
// target is not cheaper.
func SavingsRatio(from, to Model) float64 {
	fromCost := from.InputM + from.OutputM
	toCost := to.InputM + to.OutputM
	if fromCost <= 0 || toCost >= fromCost {
		return 0
	}
	return (fromCost - toCost) / fromCost
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
