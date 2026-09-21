// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package catalog

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

func TestLoad_ReturnsEmbedded(t *testing.T) {
	c := Load()
	if c == nil {
		t.Fatal("Load() returned nil")
	}
}

func TestModelFor_KnownHarness(t *testing.T) {
	c := Load()
	harnesses := []string{"claude-code", "cursor", "codex"}
	tiers := []core.Tier{core.TierSmall, core.TierMid, core.TierFrontier}

	for _, h := range harnesses {
		for _, tier := range tiers {
			m := c.ModelFor(h, tier)
			if m.ID == "" {
				t.Errorf("ModelFor(%q, %s) returned empty ID", h, tier)
			}
			if m.Tier != tier {
				t.Errorf("ModelFor(%q, %s): model tier = %s, want %s", h, tier, m.Tier, tier)
			}
		}
	}
}

func TestModelFor_UnknownHarness_FallsBack(t *testing.T) {
	c := Load()
	m := c.ModelFor("unknown-harness", core.TierSmall)
	if m.ID == "" {
		t.Error("ModelFor(unknown) must return a fallback, not empty ID")
	}
}

func TestLookupByID_Found(t *testing.T) {
	c := Load()
	// The small claude-code model must be findable by ID.
	small := c.ModelFor("claude-code", core.TierSmall)
	m, ok := c.LookupByID("claude-code", small.ID)
	if !ok {
		t.Fatalf("LookupByID(%q) = not found", small.ID)
	}
	if m.ID != small.ID {
		t.Errorf("LookupByID returned %q, want %q", m.ID, small.ID)
	}
}

func TestLookupByID_NotFound(t *testing.T) {
	c := Load()
	_, ok := c.LookupByID("claude-code", "nonexistent-xyz-12345")
	if ok {
		t.Error("LookupByID(nonexistent) should return false")
	}
}

func TestLookupByID_EmptyID(t *testing.T) {
	c := Load()
	_, ok := c.LookupByID("claude-code", "")
	if ok {
		t.Error("LookupByID('') should return false")
	}
}

// --- Family prefix (version-agnostic) matching ---

func TestLookupByID_FamilyMatchVersionBump(t *testing.T) {
	c := Load()
	// claude-opus-4-9 doesn't exist in the catalog, but starts with family
	// "claude-opus" → should match the frontier tier entry.
	m, ok := c.LookupByID("claude-code", "claude-opus-4-9")
	if !ok {
		t.Fatal("version-bumped opus ID should match via family prefix")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("family match tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_FamilyMatchHaiku(t *testing.T) {
	c := Load()
	m, ok := c.LookupByID("claude-code", "claude-haiku-5")
	if !ok {
		t.Fatal("claude-haiku-5 should match via family 'claude-haiku'")
	}
	if m.Tier != core.TierSmall {
		t.Errorf("family match tier = %s, want small", m.Tier)
	}
}

func TestLookupByID_FamilyMatchCodexFrontier(t *testing.T) {
	c := Load()
	// gpt-5.3-codex-v2 starts with family "gpt-5.3-codex" → frontier
	m, ok := c.LookupByID("codex", "gpt-5.3-codex-v2")
	if !ok {
		t.Fatal("gpt-5.3-codex-v2 should match via family")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("family match tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_FamilyMatchCaseInsensitive(t *testing.T) {
	c := Load()
	m, ok := c.LookupByID("claude-code", "Claude-Opus-5")
	if !ok {
		t.Fatal("family match should be case-insensitive")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_ExactWinsOverFamily(t *testing.T) {
	c := Load()
	// Exact ID match must take precedence over family prefix.
	// "claude-sonnet-4-6" is in catalog; if family also matched claude-sonnet,
	// the result must still be the exact entry (same tier here, but the model
	// ID must come from the exact match, not a different family hit).
	exact := c.ModelFor("claude-code", core.TierMid)
	m, ok := c.LookupByID("claude-code", exact.ID)
	if !ok {
		t.Fatal("exact ID not found")
	}
	if m.ID != exact.ID {
		t.Errorf("exact match returned wrong ID %q, want %q", m.ID, exact.ID)
	}
}

func TestSavingsRatio_CheaperTarget(t *testing.T) {
	c := Load()
	frontier := c.ModelFor("claude-code", core.TierFrontier)
	small := c.ModelFor("claude-code", core.TierSmall)
	ratio := c.SavingsRatio(frontier, small)
	if ratio <= 0 || ratio >= 1 {
		t.Errorf("SavingsRatio(frontier→small) = %.4f, want in (0,1)", ratio)
	}
}

func TestSavingsRatio_MoreExpensiveTarget(t *testing.T) {
	c := Load()
	small := c.ModelFor("claude-code", core.TierSmall)
	frontier := c.ModelFor("claude-code", core.TierFrontier)
	if r := c.SavingsRatio(small, frontier); r != 0 {
		t.Errorf("SavingsRatio(small→frontier) = %.4f, want 0", r)
	}
}

func TestEffortValue_KnownScale(t *testing.T) {
	c := Load()
	// codex uses reasoning_effort; low should map to "low"
	entry, ok := c.EntryFor("codex", c.ModelFor("codex", core.TierSmall).ID)
	if !ok {
		t.Fatal("codex small entry not found")
	}
	v := EffortValue(entry, core.EffortLow)
	if v == "" {
		t.Error("EffortValue returned empty string")
	}
}

func TestEntries_NotEmpty(t *testing.T) {
	c := Load()
	if len(c.Entries()) == 0 {
		t.Error("Entries() returned empty slice")
	}
}

func TestParseTier(t *testing.T) {
	cases := []struct {
		s    string
		want core.Tier
		ok   bool
	}{
		{"small", core.TierSmall, true},
		{"mid", core.TierMid, true},
		{"frontier", core.TierFrontier, true},
		{"SMALL", core.TierSmall, true},
		{"unknown", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseTier(tc.s)
		if ok != tc.ok {
			t.Errorf("parseTier(%q): ok=%v, want %v", tc.s, ok, tc.ok)
		}
		if ok && got != tc.want {
			t.Errorf("parseTier(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}
