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
func Route(prompt, harness, currentModelID string) Decision {
	cls := Classify(prompt)
	tier := cls.Complexity.Tier()
	recommended := ModelFor(harness, tier)

	d := Decision{
		Complexity: cls.Complexity,
		Tier:       tier,
		Harness:    harness,
		Model:      recommended,
		Confident:  cls.Confident,
	}

	current, known := lookupCurrent(harness, currentModelID)
	if !known {
		d.Verdict = VerdictUnknown
		return d
	}
	d.CurrentModel = current

	switch {
	case current.Tier == tier:
		d.Verdict = VerdictOK
	case current.Tier > tier:
		// Current is stronger than needed — downshift to save money.
		d.Verdict = VerdictDownshift
		d.Savings = SavingsRatio(current, recommended)
	default:
		// Current is weaker than needed — upshift for quality.
		d.Verdict = VerdictUpshift
	}
	return d
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
