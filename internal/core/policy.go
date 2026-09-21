// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

import "fmt"

// Tier maps a complexity to the minimum capable tier — the gearbox rule:
// which gear does this stretch of road need? Lives here because it is routing
// policy, not classification knowledge.
func (c Complexity) Tier() Tier {
	switch c {
	case Trivial:
		return TierSmall // flat straight: high gear, cheap
	case Simple:
		return TierMid
	case Medium:
		return TierMid
	case Complex:
		return TierFrontier // sharp curve: downshift for torque
	default:
		return TierMid
	}
}

// Decision is the full routing decision for one subagent task: what the task
// needs, what model to use on the given harness, and how it compares to the
// model the harness would have used by default.
type Decision struct {
	Complexity   Complexity
	Tier         Tier
	Effort       Effort // recommended reasoning intensity for this tier
	Harness      string
	Model        Model // the model we recommend for this task
	CurrentModel Model // the model currently in effect (may be zero if unknown)
	Verdict      Verdict
	Savings      float64 // fraction cheaper vs current (0 if not cheaper / unknown)
	Confident    bool
}

// Verdict tells the caller what to do about the current model.
type Verdict int

const (
	// VerdictOK — the current model already matches the needed tier.
	VerdictOK Verdict = iota
	// VerdictDownshift — the current model is stronger (and pricier) than needed.
	VerdictDownshift
	// VerdictUpshift — the current model is weaker than the task needs.
	VerdictUpshift
	// VerdictUnknown — no current model known; we only state the recommendation.
	VerdictUnknown
)

// String returns the human-readable verdict.
func (v Verdict) String() string {
	switch v {
	case VerdictOK:
		return "OK"
	case VerdictDownshift:
		return "DOWNSHIFT"
	case VerdictUpshift:
		return "UPSHIFT"
	default:
		return "UNKNOWN"
	}
}

// Route is the main entry point: given a task prompt, a harness, the current
// model id (may be empty), and an optional Resolver, produce a routing Decision.
//
// When r is nil, Route falls back to the in-package Catalog for backward
// compatibility. Once all callers pass a real Resolver (Wave 4.4), the Catalog
// var will be removed from core.
func Route(prompt, harness, currentModelID string, r ...Resolver) Decision {
	var res Resolver
	if len(r) > 0 && r[0] != nil {
		res = r[0]
	}

	tier, cls := classifyTask(prompt)
	effort := EffortFor(tier)
	recommended := resolveModel(harness, tier, res)
	verdict, savings, current := compareToCurrentModel(harness, currentModelID, recommended, res)

	return Decision{
		Complexity:   cls,
		Tier:         tier,
		Effort:       effort,
		Harness:      harness,
		Model:        recommended,
		CurrentModel: current,
		Verdict:      verdict,
		Savings:      savings,
		Confident:    Classify(prompt).Confident,
	}
}

// classifyTask scores the prompt and returns the target tier and complexity.
func classifyTask(prompt string) (Tier, Complexity) {
	cls := Classify(prompt)
	return cls.Complexity.Tier(), cls.Complexity
}

// resolveModel returns the catalog model for the given harness and tier.
// Uses the Resolver when provided; panics if nil (callers must inject one).
func resolveModel(harness string, tier Tier, r Resolver) Model {
	if r != nil {
		return r.ModelFor(harness, tier)
	}
	// No resolver — return a zero Model with the tier set; caller handles gracefully.
	return Model{Tier: tier, Harness: harness}
}

// compareToCurrentModel looks up the current model and determines the verdict.
func compareToCurrentModel(harness, currentModelID string, recommended Model, r Resolver) (Verdict, float64, Model) {
	if r == nil || currentModelID == "" {
		return VerdictUnknown, 0, Model{}
	}

	current, known := r.LookupByID(harness, currentModelID)
	if !known {
		return VerdictUnknown, 0, Model{}
	}

	switch {
	case current.Tier == recommended.Tier:
		return VerdictOK, 0, current
	case current.Tier > recommended.Tier:
		return VerdictDownshift, r.SavingsRatio(current, recommended), current
	default:
		return VerdictUpshift, 0, current
	}
}

// Summary renders a one-line, human-readable decision (the gearbox readout).
func (d Decision) Summary() string {
	switch d.Verdict {
	case VerdictDownshift:
		return fmt.Sprintf("%s task → downshift to %s (~%.0f%% cheaper)",
			d.Complexity, d.Model.ID, d.Savings*100)
	case VerdictUpshift:
		return fmt.Sprintf("%s task → upshift to %s (needs more torque)",
			d.Complexity, d.Model.ID)
	case VerdictOK:
		return fmt.Sprintf("%s task → %s (right gear, no change)",
			d.Complexity, d.Model.ID)
	default:
		return fmt.Sprintf("%s task → use %s (%s tier)",
			d.Complexity, d.Model.ID, d.Tier)
	}
}
