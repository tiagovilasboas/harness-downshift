// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package core

import (
	"strings"
	"testing"
)

// catalogID is a test helper that reads the current model ID from the catalog
// for a given harness and tier. Tests use this instead of hardcoding strings
// like "claude-haiku-4" — so they break when routing logic breaks, not when
// a model is renamed.
func catalogID(harness string, tier Tier) string {
	return Catalog[harness][tier].ID
}

// --- Route verdict tests (assert Verdict + Tier, never Model.ID) ---

func TestRoute_Downshift(t *testing.T) {
	// Trivial task on frontier → must downshift to small tier.
	frontierID := catalogID("claude-code", TierFrontier)
	d := Route("rename the variable", "claude-code", frontierID)
	if d.Verdict != VerdictDownshift {
		t.Fatalf("verdict = %s, want DOWNSHIFT", d.Verdict)
	}
	if d.Tier != TierSmall {
		t.Errorf("recommended tier = %s, want small", d.Tier)
	}
	if d.Savings <= 0 {
		t.Errorf("savings = %.2f, want > 0 on downshift", d.Savings)
	}
}

func TestRoute_Upshift(t *testing.T) {
	// Complex task on small model → must upshift to frontier tier.
	smallID := catalogID("claude-code", TierSmall)
	d := Route("rearchitect the payment system across services", "claude-code", smallID)
	if d.Verdict != VerdictUpshift {
		t.Fatalf("verdict = %s, want UPSHIFT", d.Verdict)
	}
	if d.Tier != TierFrontier {
		t.Errorf("recommended tier = %s, want frontier", d.Tier)
	}
}

func TestRoute_OK(t *testing.T) {
	// Medium task on mid model → already the right gear.
	midID := catalogID("claude-code", TierMid)
	d := Route("implement the CSV export feature", "claude-code", midID)
	if d.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", d.Verdict)
	}
	if d.Savings != 0 {
		t.Errorf("savings = %.2f, want 0 for OK", d.Savings)
	}
}

func TestRoute_UnknownCurrentModel(t *testing.T) {
	// Empty model ID → VerdictUnknown, but tier is still correctly resolved.
	d := Route("rename the variable", "claude-code", "")
	if d.Verdict != VerdictUnknown {
		t.Fatalf("verdict = %s, want UNKNOWN", d.Verdict)
	}
	if d.Tier != TierSmall {
		t.Errorf("recommended tier = %s, want small (trivial task)", d.Tier)
	}
}

func TestRoute_Codex_Downshift(t *testing.T) {
	// Trivial task on codex frontier → downshift to small tier.
	frontierID := catalogID("codex", TierFrontier)
	d := Route("fix a typo", "codex", frontierID)
	if d.Verdict != VerdictDownshift {
		t.Fatalf("verdict = %s, want DOWNSHIFT", d.Verdict)
	}
	if d.Tier != TierSmall {
		t.Errorf("recommended tier = %s, want small", d.Tier)
	}
}

func TestRoute_UnknownHarness(t *testing.T) {
	// Unknown harness falls back to claude-code catalog; verdict reflects tier match.
	d := Route("rename the variable", "unknown-harness", "")
	// Model must still be resolved (fallback to claude-code small for trivial task).
	if d.Tier != TierSmall {
		t.Errorf("recommended tier = %s, want small", d.Tier)
	}
	// No current model → verdict must be UNKNOWN.
	if d.Verdict != VerdictUnknown {
		t.Errorf("verdict = %s, want UNKNOWN (no current model)", d.Verdict)
	}
}

// --- SavingsRatio contract tests (math only, no hardcoded costs) ---

func TestSavingsRatio_CheaperTarget(t *testing.T) {
	// Frontier is always more expensive than small; savings must be in (0, 1).
	frontier := Catalog["claude-code"][TierFrontier]
	small := Catalog["claude-code"][TierSmall]
	got := SavingsRatio(frontier, small)
	if got <= 0 || got >= 1 {
		t.Errorf("SavingsRatio(frontier→small) = %.4f, want in (0, 1)", got)
	}
}

func TestSavingsRatio_MoreExpensiveTarget(t *testing.T) {
	// Upshift should never show savings.
	small := Catalog["claude-code"][TierSmall]
	frontier := Catalog["claude-code"][TierFrontier]
	if r := SavingsRatio(small, frontier); r != 0 {
		t.Errorf("SavingsRatio(small→frontier) = %.4f, want 0", r)
	}
}

// --- Decision.Summary contract tests (shape, not specific model names) ---

func TestDecisionSummary_Downshift(t *testing.T) {
	frontierID := catalogID("claude-code", TierFrontier)
	d := Route("rename the variable", "claude-code", frontierID)
	s := d.Summary()
	if s == "" {
		t.Fatal("Summary() must not be empty")
	}
	if !strings.Contains(strings.ToLower(s), "downshift") {
		t.Errorf("Summary() = %q, want 'downshift' in text", s)
	}
}

func TestDecisionSummary_Upshift(t *testing.T) {
	smallID := catalogID("claude-code", TierSmall)
	d := Route("rearchitect the payment system across services", "claude-code", smallID)
	s := d.Summary()
	if !strings.Contains(strings.ToLower(s), "upshift") {
		t.Errorf("Summary() = %q, want 'upshift' in text", s)
	}
}

func TestDecisionSummary_OK(t *testing.T) {
	midID := catalogID("claude-code", TierMid)
	d := Route("implement the CSV export feature", "claude-code", midID)
	s := d.Summary()
	if !strings.Contains(strings.ToLower(s), "right gear") {
		t.Errorf("Summary() = %q, want 'right gear' in text", s)
	}
}
