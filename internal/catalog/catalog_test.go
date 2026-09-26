// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package catalog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestModelFor_KnownIncompleteHarnessDoesNotBorrowClaudeModel(t *testing.T) {
	c, err := parse([]byte(`{
		"entries": [
			{"id":"claude-haiku","harness":"claude-code","tier":"small"},
			{"id":"codex-terra","harness":"codex","tier":"mid"}
		]
	}`))
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}

	m := c.ModelFor("codex", core.TierSmall)
	if m.ID != "" {
		t.Fatalf("known incomplete harness returned model %q; must not borrow another harness model", m.ID)
	}
	if m.Harness != "codex" {
		t.Errorf("zero model harness = %q, want codex", m.Harness)
	}
	if m.Tier != core.TierSmall {
		t.Errorf("zero model tier = %s, want small", m.Tier)
	}
}

func TestModelFor_DeclaredHarnessWithoutRoutableModelsDoesNotFallback(t *testing.T) {
	c, err := parse([]byte(`{
		"entries": [
			{"id":"claude-haiku","harness":"claude-code","tier":"small"},
			{"id":"pending-model","harness":"codex","tier":"unknown"}
		]
	}`))
	if err != nil {
		t.Fatalf("parse catalog: %v", err)
	}

	m := c.ModelFor("codex", core.TierSmall)
	if m.ID != "" {
		t.Fatalf("declared harness without routable tiers returned model %q; must not fallback", m.ID)
	}
	if m.Harness != "codex" {
		t.Errorf("zero model harness = %q, want codex", m.Harness)
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
	// gpt-5.6-sol-v2 starts with family "gpt-5.6-sol" → frontier
	m, ok := c.LookupByID("codex", "gpt-5.6-sol-v2")
	if !ok {
		t.Fatal("gpt-5.6-sol-v2 should match via family")
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
	exact := c.ModelFor("claude-code", core.TierMid)
	m, ok := c.LookupByID("claude-code", exact.ID)
	if !ok {
		t.Fatal("exact ID not found")
	}
	if m.ID != exact.ID {
		t.Errorf("exact match returned wrong ID %q, want %q", m.ID, exact.ID)
	}
}

// --- OpenRouter "provider/model-id" normalisation ---

func TestLookupByID_OpenRouterExactMatch(t *testing.T) {
	c := Load()
	// "anthropic/claude-opus-4-8" → strip prefix → exact match on "claude-opus-4-8"
	m, ok := c.LookupByID("claude-code", "anthropic/claude-opus-4-8")
	if !ok {
		t.Fatal("OpenRouter-prefixed exact ID should match after normalisation")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_OpenRouterFamilyMatch(t *testing.T) {
	c := Load()
	// "anthropic/claude-opus-4-9" → strip → "claude-opus-4-9" → family "claude-opus"
	m, ok := c.LookupByID("claude-code", "anthropic/claude-opus-4-9")
	if !ok {
		t.Fatal("OpenRouter-prefixed version-bumped ID should match via family")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_OpenRouterHaikuSmall(t *testing.T) {
	c := Load()
	m, ok := c.LookupByID("claude-code", "anthropic/claude-haiku-5")
	if !ok {
		t.Fatal("anthropic/claude-haiku-5 should match via family")
	}
	if m.Tier != core.TierSmall {
		t.Errorf("tier = %s, want small", m.Tier)
	}
}

func TestLookupByID_OpenRouterCodex(t *testing.T) {
	c := Load()
	// "openai/gpt-5.6-sol" → strip → exact match
	m, ok := c.LookupByID("codex", "openai/gpt-5.6-sol")
	if !ok {
		t.Fatal("openai/gpt-5.6-sol should match after normalisation")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_OpenRouterUnknownProvider(t *testing.T) {
	c := Load()
	// "somevendor/claude-haiku-5" → strip → "claude-haiku-5" → family match
	m, ok := c.LookupByID("claude-code", "somevendor/claude-haiku-5")
	if !ok {
		t.Fatal("any-provider/claude-haiku-5 should match via family after normalisation")
	}
	if m.Tier != core.TierSmall {
		t.Errorf("tier = %s, want small", m.Tier)
	}
}

func TestLookupByID_OpenRouterTrulyUnknown(t *testing.T) {
	c := Load()
	_, ok := c.LookupByID("claude-code", "somevendor/total-unknown-xyz-12345")
	if ok {
		t.Error("genuinely unknown model should return false even with provider prefix")
	}
}

// --- Grok installed in non-native harness ---

func TestLookupByID_GrokInCursor(t *testing.T) {
	c := Load()
	m, ok := c.LookupByID("cursor", "grok-4.6")
	if !ok {
		t.Fatal("grok-4.6 should be found in cursor catalog")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("grok in cursor tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_GrokVersionBumpInCursor(t *testing.T) {
	c := Load()
	// grok-4.7 not in catalog → family "grok" → frontier
	m, ok := c.LookupByID("cursor", "grok-4.7")
	if !ok {
		t.Fatal("grok-4.7 should match via family 'grok' in cursor")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("tier = %s, want frontier", m.Tier)
	}
}

func TestLookupByID_GrokInClaudeCode(t *testing.T) {
	c := Load()
	m, ok := c.LookupByID("claude-code", "grok-4.6")
	if !ok {
		t.Fatal("grok-4.6 should be found in claude-code catalog")
	}
	if m.Tier != core.TierFrontier {
		t.Errorf("grok in claude-code tier = %s, want frontier", m.Tier)
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

// The README cost claims (Quickstart "~80% cheaper" and the Cost evidence
// table) are computed from these embedded entries. If a price changes, this
// test fails so the README numbers are recomputed in the same change.
func TestEmbeddedClaudeCodeSavingsMatchesREADME(t *testing.T) {
	c, err := parse(embeddedJSON)
	if err != nil {
		t.Fatal(err)
	}
	frontier := c.ModelFor("claude-code", core.TierFrontier)
	small := c.ModelFor("claude-code", core.TierSmall)
	if frontier.ID != "claude-opus-4-8" || small.ID != "claude-haiku-4-5" {
		t.Fatalf("claude-code frontier/small = %s/%s, want claude-opus-4-8/claude-haiku-4-5", frontier.ID, small.ID)
	}
	// (5+25 - (1+5)) / (5+25) = 0.80
	if got := c.SavingsRatio(frontier, small); got < 0.7999 || got > 0.8001 {
		t.Errorf("SavingsRatio(frontier→small) = %.4f, want 0.80 (README says ~80%%)", got)
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

// --- explicit_only routing policy ---

func TestIsExplicitOnly_AstraIsExplicitOnly(t *testing.T) {
	c := Load()
	if !c.IsExplicitOnly("codex", "gpt-6-astra") {
		t.Error("gpt-6-astra should be explicit_only")
	}
}

func TestIsExplicitOnly_NormalModelIsNotExplicit(t *testing.T) {
	c := Load()
	frontier := c.ModelFor("codex", core.TierFrontier)
	if c.IsExplicitOnly("codex", frontier.ID) {
		t.Errorf("%s should not be explicit_only", frontier.ID)
	}
}

func TestIsExplicitOnly_EmptyIDReturnsFalse(t *testing.T) {
	c := Load()
	if c.IsExplicitOnly("codex", "") {
		t.Error("empty model ID should return false")
	}
}

func TestIsExplicitOnly_UnknownIDReturnsFalse(t *testing.T) {
	c := Load()
	if c.IsExplicitOnly("codex", "completely-unknown-model") {
		t.Error("unknown model should return false (fail-open)")
	}
}

func TestAstraNotChosenAsAutomaticTarget(t *testing.T) {
	c := Load()
	// gpt-6-astra is explicit_only — ModelFor must NEVER return it as the
	// automatic target for any tier, even if it is the only frontier model
	// registered for that harness.
	for _, tier := range []core.Tier{core.TierSmall, core.TierMid, core.TierFrontier} {
		m := c.ModelFor("codex", tier)
		if m.ID == "gpt-6-astra" {
			t.Errorf("ModelFor(codex, %s) returned explicit_only model gpt-6-astra", tier)
		}
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

func TestParse_RejectsDuplicateModelOrAlias(t *testing.T) {
	data := []byte(`{"entries":[{"id":"first","harness":"codex","tier":"small"},{"id":"second","aliases":["first"],"harness":"codex","tier":"mid"}]}`)
	if _, err := parse(data); err == nil {
		t.Fatal("parse accepted duplicate model ID/alias")
	}
}

func TestMergeEntries_PreservesDefaultsAndOverridesTarget(t *testing.T) {
	base := []Entry{{ID: "luna", Harness: "codex", Tier: "small"}, {ID: "terra", Harness: "codex", Tier: "mid"}}
	override := []Entry{{ID: "terra", Harness: "codex", Tier: "frontier"}, {ID: "custom", Harness: "codex", Tier: "frontier"}}
	merged := mergeEntries(base, override)
	encoded, err := json.Marshal(merged)
	if err != nil {
		t.Fatalf("marshal merged: %v", err)
	}
	if string(encoded) == "" || len(merged) != 3 || merged[0].ID != "luna" || merged[1].Tier != "frontier" {
		t.Fatalf("merge did not preserve and override entries: %#v", merged)
	}
}

func TestLoad_WarnsAndFallsBackWhenOverrideCannotBeRead(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".harness-downshift", "catalog.json")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll override directory: %v", err)
	}

	var warnings bytes.Buffer
	c := load(&warnings)
	if c == nil || len(c.Entries()) == 0 {
		t.Fatal("load() did not fall back to the embedded catalog")
	}
	if !strings.Contains(warnings.String(), "could not read") {
		t.Errorf("warnings = %q, want override read warning", warnings.String())
	}
}

// --- load: path where override parses but fails validateEntries (could not merge) ---

func TestLoad_WarnsAndFallsBackWhenMergeFails(t *testing.T) {
	// Write a catalog where an override entry adds a new model whose family
	// prefix overlaps with an existing family in the embedded base, causing
	// validateEntries(merged) to fail with "overlapping model families".
	// The base has "claude-haiku" as a family for claude-code.
	// If we add a new entry with family "claude" (a prefix of "claude-haiku"),
	// the merged validation rejects it.
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".harness-downshift")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// A new ID that won't collide in the override alone, but whose family "claude"
	// is a prefix of "claude-haiku" in the base → merged validation fails.
	overlap := `{"version":"1","entries":[
		{"id":"claude-new-model","family":"claude","harness":"claude-code","tier":"small"}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(overlap), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var warnings bytes.Buffer
	c := load(&warnings)
	if c == nil || len(c.Entries()) == 0 {
		t.Fatal("load() must fall back to embedded catalog on merge failure")
	}
	w := warnings.String()
	// Either "could not merge" (from validateEntries) or "could not parse" (if
	// decode rejects the overlap earlier). Either path means fall-back worked.
	if !strings.Contains(w, "could not merge") && !strings.Contains(w, "could not parse") {
		t.Errorf("warnings = %q, want warning about merge or parse failure", w)
	}
}

// --- Catalog.EffortFor (method) ---

func TestCatalogEffortFor_KnownModel(t *testing.T) {
	c := Load()
	// gpt-5.6-sol is the codex frontier; its effort_map maps "low" → "low"
	frontierID := c.ModelFor("codex", core.TierFrontier).ID
	got := c.EffortFor("codex", frontierID, core.EffortLow)
	if got == "" {
		t.Error("EffortFor must return a non-empty effort string for a known model")
	}
}

func TestCatalogEffortFor_UnknownModel_Fallback(t *testing.T) {
	c := Load()
	// Unknown model → falls back to effort.String()
	got := c.EffortFor("codex", "completely-unknown-xyz", core.EffortHigh)
	if got != "high" {
		t.Errorf("EffortFor unknown model = %q, want 'high' (effort.String() fallback)", got)
	}
}

// --- EffortValue: entry with no effort_map key for the requested level ---

func TestEffortValue_MissingKey_FallsBackToEffortString(t *testing.T) {
	// An entry with an empty effort_map has no keys at all.
	e := Entry{EffortMap: map[string]string{}}
	got := EffortValue(e, core.EffortMid)
	if got != "medium" {
		t.Errorf("EffortValue with empty map = %q, want 'medium' (effort.String())", got)
	}
}

func TestEffortValue_NilEffortMap_FallsBackToEffortString(t *testing.T) {
	e := Entry{} // EffortMap is nil
	got := EffortValue(e, core.EffortHigh)
	if got != "high" {
		t.Errorf("EffortValue with nil map = %q, want 'high'", got)
	}
}
