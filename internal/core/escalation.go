// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

import "strings"

// EscalationIntent is a named task intent that sits above raw complexity.
// It captures the *purpose* of the work, not just its difficulty, and drives
// the recommended tier and effort without adapter-specific if-chains.
//
// Four intents cover all cases:
//
//	TrivialIntent   — mechanical work: rename, format, git ops, typo fix.
//	                  Always small/low. No reasoning depth needed.
//	NormalIntent    — everyday implementation: add a field, fix a bug,
//	                  refactor a module. Mid/medium is the right gear.
//	ReviewIntent    — deep analysis without heavy code production: code review,
//	                  security audit, RFC, architecture exploration. Uses the
//	                  frontier tier and high effort because the agent must reason
//	                  extensively over existing code — not because it will write much.
//	PreservedIntent — the current model is marked explicit_only (e.g. the user
//	                  deliberately picked a specialised top model). The router
//	                  must not replace it. Tier and effort are read from the
//	                  Decision, not from the intent.
type EscalationIntent int

const (
	TrivialIntent   EscalationIntent = iota // small tier, low effort
	NormalIntent                            // mid tier, medium effort
	ReviewIntent                            // frontier tier, high effort
	PreservedIntent                         // keep current model as-is
)

// String returns the human-readable intent name.
func (i EscalationIntent) String() string {
	switch i {
	case TrivialIntent:
		return "trivial"
	case NormalIntent:
		return "normal"
	case ReviewIntent:
		return "review"
	case PreservedIntent:
		return "preserved"
	default:
		return "unknown"
	}
}

// Tier returns the recommended model tier for this intent.
// For PreservedIntent the caller must use the Decision's existing model tier.
func (i EscalationIntent) Tier() Tier {
	switch i {
	case TrivialIntent:
		return TierSmall
	case ReviewIntent:
		return TierFrontier
	case PreservedIntent:
		return TierFrontier // preserve means frontier-or-above; caller checks explicit model
	default: // NormalIntent and unknown
		return TierMid
	}
}

// Effort returns the recommended reasoning effort for this intent.
// For PreservedIntent the caller must use the Decision's existing effort.
func (i EscalationIntent) Effort() Effort {
	switch i {
	case TrivialIntent:
		return EffortLow
	case ReviewIntent, PreservedIntent:
		return EffortHigh
	default: // NormalIntent and unknown
		return EffortMid
	}
}

// reviewSignals is the set of complexity signals that, when dominant, indicate
// a deep-analysis task that deserves the ReviewIntent escalation.
// These are a subset of Complex signals — the ones that imply reading and
// reasoning over existing code rather than writing new code.
var reviewSignals = map[string]bool{
	"code review":     true,
	"security audit":  true,
	"security review": true,
	"rfc":             true,
	"design doc":      true,
	"think through":   true,
	"trade-off":       true,
	"tradeoff":        true,
	"tradeoffs":       true,
}

// isReviewTask returns true when the prompt is dominated by deep-analysis
// signals rather than construction signals.
func isReviewTask(prompt string, cls Classification) bool {
	if cls.Complexity != Complex {
		return false
	}
	lower := strings.ToLower(prompt)
	for phrase := range reviewSignals {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// IntentFor maps a completed Decision to the appropriate EscalationIntent.
//
// The mapping in priority order:
//  1. Preserved  — current model is explicit_only (user's deliberate choice).
//  2. Review     — Complex task dominated by deep-analysis signals.
//  3. Trivial    — Trivial complexity.
//  4. Normal     — everything else (Simple, Medium, Complex construction).
func IntentFor(prompt string, cls Classification, d Decision, r Resolver) EscalationIntent {
	// 1. Explicit preservation beats everything.
	if r != nil && d.CurrentModel.ID != "" {
		if r.IsExplicitOnly(d.Harness, d.CurrentModel.ID) {
			return PreservedIntent
		}
	}

	switch cls.Complexity {
	case Trivial:
		return TrivialIntent
	case Complex:
		if isReviewTask(prompt, cls) {
			return ReviewIntent
		}
		return NormalIntent // complex construction → normal (frontier, but building)
	default: // Simple, Medium
		return NormalIntent
	}
}

