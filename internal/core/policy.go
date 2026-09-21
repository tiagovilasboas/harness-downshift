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

// Route is the main entry point: given a task prompt, a harness, and the
// current model id (may be empty when unknown), produce a routing Decision.
// It composes three focused steps: classify, resolve, compare.
func Route(prompt, harness, currentModelID string) Decision {
	tier, cls := classifyTask(prompt)
	recommended := resolveModel(harness, tier)
	verdict, savings, current := compareToCurrentModel(harness, currentModelID, recommended)

	return Decision{
		Complexity:   cls,
		Tier:         tier,
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
func resolveModel(harness string, tier Tier) Model {
	return ModelFor(harness, tier)
}

// compareToCurrentModel looks up the current model in the catalog and
// determines whether to downshift, upshift, keep, or flag as unknown.
func compareToCurrentModel(harness, currentModelID string, recommended Model) (Verdict, float64, Model) {
	current, known := lookupCurrent(harness, currentModelID)
	if !known {
		return VerdictUnknown, 0, Model{}
	}

	switch {
	case current.Tier == recommended.Tier:
		return VerdictOK, 0, current
	case current.Tier > recommended.Tier:
		return VerdictDownshift, SavingsRatio(current, recommended), current
	default:
		return VerdictUpshift, 0, current
	}
}

// lookupCurrent resolves a current model id to a catalog Model. Returns
// known=false when the id is empty or not recognized in the harness catalog.
func lookupCurrent(harness, modelID string) (Model, bool) {
	if modelID == "" {
		return Model{}, false
	}
	models, ok := Catalog[harness]
	if !ok {
		return Model{}, false
	}
	for _, m := range models {
		if m.ID == modelID {
			return m, true
		}
	}
	return Model{}, false
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
