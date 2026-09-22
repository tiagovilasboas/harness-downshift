// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package domain

import "github.com/tiagovilasboas/harness-downshift/internal/core"

// ModelProfile describes a model's capabilities and cost.
// This is derived from catalog entries but kept separate so the routing
// logic doesn't depend on the catalog package directly.
type ModelProfile struct {
	ID           string       // model identifier
	Harness      string       // which harness this belongs to
	Tier         core.Tier    // capability/cost bucket
	Capabilities Capabilities // what this model can do
	InputCost    float64      // USD per 1M input tokens
	OutputCost   float64      // USD per 1M output tokens
	ExplicitOnly bool         // if true, never auto-select as routing target
}

// Capabilities describes what a model can do, in [0, 1] scale.
// Higher values mean better capability in that dimension.
type Capabilities struct {
	Coding      float64 `json:"coding"`
	Reasoning   float64 `json:"reasoning"`
	Planning    float64 `json:"planning"`
	Debugging   float64 `json:"debugging"`
	Security    float64 `json:"security"`
	ToolUse     float64 `json:"tool_use"`
	LongContext float64 `json:"long_context"`
}

// Satisfies returns true if this model's capabilities meet or exceed the requirements.
func (c Capabilities) Satisfies(req CapabilityReq) bool {
	return c.Coding >= req.Coding &&
		c.Reasoning >= req.Reasoning &&
		c.Planning >= req.Planning &&
		c.Debugging >= req.Debugging &&
		c.Security >= req.Security &&
		c.ToolUse >= req.ToolUse &&
		c.LongContext >= req.LongContext
}

// DefaultCapabilities returns default capability values based on tier.
// Used when the catalog entry doesn't specify capabilities explicitly.
func DefaultCapabilities(tier core.Tier) Capabilities {
	switch tier {
	case core.TierSmall:
		return Capabilities{
			Coding:      0.4,
			Reasoning:   0.4,
			Planning:    0.4,
			Debugging:   0.4,
			Security:    0.4,
			ToolUse:     0.4,
			LongContext: 0.4,
		}
	case core.TierMid:
		return Capabilities{
			Coding:      0.7,
			Reasoning:   0.7,
			Planning:    0.7,
			Debugging:   0.7,
			Security:    0.7,
			ToolUse:     0.7,
			LongContext: 0.7,
		}
	case core.TierFrontier:
		return Capabilities{
			Coding:      0.95,
			Reasoning:   0.95,
			Planning:    0.95,
			Debugging:   0.95,
			Security:    0.95,
			ToolUse:     0.95,
			LongContext: 0.95,
		}
	default:
		return DefaultCapabilities(core.TierMid)
	}
}

// TotalCost returns a simple cost estimate (input + output) for comparison.
func (p ModelProfile) TotalCost() float64 {
	return p.InputCost + p.OutputCost
}
