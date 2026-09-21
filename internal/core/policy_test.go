// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core_test

import (
	"strings"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// cat is the test-scope resolver — same embedded catalog the binary uses.
var cat = catalog.Load()

// catID reads the current model ID from the catalog for a given harness+tier.
func catID(harness string, tier core.Tier) string {
	return cat.ModelFor(harness, tier).ID
}

// --- Route verdict tests ---

func TestRoute_Downshift(t *testing.T) {
	frontierID := catID("claude-code", core.TierFrontier)
	d := core.Route("rename the variable", "claude-code", frontierID, cat)
	if d.Verdict != core.VerdictDownshift {
		t.Fatalf("verdict = %s, want DOWNSHIFT", d.Verdict)
	}
	if d.Tier != core.TierSmall {
		t.Errorf("recommended tier = %s, want small", d.Tier)
	}
	if d.Savings <= 0 {
		t.Errorf("savings = %.2f, want > 0", d.Savings)
	}
}

func TestRoute_Upshift(t *testing.T) {
	smallID := catID("claude-code", core.TierSmall)
	d := core.Route("rearchitect the payment system across services", "claude-code", smallID, cat)
	if d.Verdict != core.VerdictUpshift {
		t.Fatalf("verdict = %s, want UPSHIFT", d.Verdict)
	}
	if d.Tier != core.TierFrontier {
		t.Errorf("recommended tier = %s, want frontier", d.Tier)
	}
}

func TestRoute_OK(t *testing.T) {
	midID := catID("claude-code", core.TierMid)
	d := core.Route("implement the CSV export feature", "claude-code", midID, cat)
	if d.Verdict != core.VerdictOK {
		t.Fatalf("verdict = %s, want OK", d.Verdict)
	}
	if d.Savings != 0 {
		t.Errorf("savings = %.2f, want 0 for OK", d.Savings)
	}
}

func TestRoute_UnknownCurrentModel(t *testing.T) {
	d := core.Route("rename the variable", "claude-code", "", cat)
	if d.Verdict != core.VerdictUnknown {
		t.Fatalf("verdict = %s, want UNKNOWN", d.Verdict)
	}
	if d.Tier != core.TierSmall {
		t.Errorf("recommended tier = %s, want small", d.Tier)
	}
}

func TestRoute_Codex_Downshift(t *testing.T) {
	frontierID := catID("codex", core.TierFrontier)
	d := core.Route("fix a typo", "codex", frontierID, cat)
	if d.Verdict != core.VerdictDownshift {
		t.Fatalf("verdict = %s, want DOWNSHIFT", d.Verdict)
	}
	if d.Tier != core.TierSmall {
		t.Errorf("recommended tier = %s, want small", d.Tier)
	}
}

func TestRoute_UnknownHarness(t *testing.T) {
	d := core.Route("rename the variable", "unknown-harness", "", cat)
	if d.Tier != core.TierSmall {
		t.Errorf("recommended tier = %s, want small", d.Tier)
	}
	if d.Verdict != core.VerdictUnknown {
		t.Errorf("verdict = %s, want UNKNOWN", d.Verdict)
	}
}

// --- Effort field tests ---

func TestRoute_EffortSetOnDecision(t *testing.T) {
	cases := []struct {
		prompt     string
		wantEffort core.Effort
	}{
		{"rename the variable", core.EffortLow},
		{"implement the CSV export feature", core.EffortMid},
		{"rearchitect the payment system across services", core.EffortHigh},
	}
	for _, tc := range cases {
		d := core.Route(tc.prompt, "claude-code", "", cat)
		if d.Effort != tc.wantEffort {
			t.Errorf("Route(%q).Effort = %s, want %s", tc.prompt, d.Effort, tc.wantEffort)
		}
	}
}

// --- SavingsRatio contract tests ---

func TestSavingsRatio_CheaperTarget(t *testing.T) {
	frontier := cat.ModelFor("claude-code", core.TierFrontier)
	small := cat.ModelFor("claude-code", core.TierSmall)
	got := cat.SavingsRatio(frontier, small)
	if got <= 0 || got >= 1 {
		t.Errorf("SavingsRatio(frontier→small) = %.4f, want in (0,1)", got)
	}
}

func TestSavingsRatio_MoreExpensiveTarget(t *testing.T) {
	small := cat.ModelFor("claude-code", core.TierSmall)
	frontier := cat.ModelFor("claude-code", core.TierFrontier)
	if r := cat.SavingsRatio(small, frontier); r != 0 {
		t.Errorf("SavingsRatio(small→frontier) = %.4f, want 0", r)
	}
}

// --- Summary contract tests ---

func TestDecisionSummary_Downshift(t *testing.T) {
	d := core.Route("rename the variable", "claude-code", catID("claude-code", core.TierFrontier), cat)
	s := d.Summary()
	if !strings.Contains(strings.ToLower(s), "downshift") {
		t.Errorf("Summary() = %q, want 'downshift'", s)
	}
}

func TestDecisionSummary_Upshift(t *testing.T) {
	d := core.Route("rearchitect the payment system", "claude-code", catID("claude-code", core.TierSmall), cat)
	s := d.Summary()
	if !strings.Contains(strings.ToLower(s), "upshift") {
		t.Errorf("Summary() = %q, want 'upshift'", s)
	}
}

func TestDecisionSummary_OK(t *testing.T) {
	d := core.Route("implement the CSV export feature", "claude-code", catID("claude-code", core.TierMid), cat)
	s := d.Summary()
	if !strings.Contains(strings.ToLower(s), "right gear") {
		t.Errorf("Summary() = %q, want 'right gear'", s)
	}
}
