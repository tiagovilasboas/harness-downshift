package core

import (
	"math"
	"testing"
)

func TestRoute_Downshift(t *testing.T) {
	// Trivial task on a frontier model → downshift.
	d := Route("rename the variable", "claude-code", "claude-opus-4-8")
	if d.Verdict != VerdictDownshift {
		t.Fatalf("verdict = %s, want DOWNSHIFT", d.Verdict)
	}
	if d.Model.ID != "claude-haiku-4" {
		t.Errorf("recommended = %s, want claude-haiku-4", d.Model.ID)
	}
	// opus (15+75=90) → haiku (0.8+4=4.8): ~94% cheaper
	if d.Savings < 0.9 {
		t.Errorf("savings = %.2f, want >= 0.9", d.Savings)
	}
}

func TestRoute_Upshift(t *testing.T) {
	// Complex task on a small model → upshift.
	d := Route("rearchitect the payment system across services", "claude-code", "claude-haiku-4")
	if d.Verdict != VerdictUpshift {
		t.Fatalf("verdict = %s, want UPSHIFT", d.Verdict)
	}
	if d.Model.ID != "claude-opus-4-8" {
		t.Errorf("recommended = %s, want claude-opus-4-8", d.Model.ID)
	}
}

func TestRoute_OK(t *testing.T) {
	// Medium task on a mid model → OK.
	d := Route("implement the CSV export feature", "claude-code", "claude-sonnet-4-6")
	if d.Verdict != VerdictOK {
		t.Fatalf("verdict = %s, want OK", d.Verdict)
	}
	if d.Savings != 0 {
		t.Errorf("savings = %.2f, want 0 for OK", d.Savings)
	}
}

func TestRoute_UnknownCurrentModel(t *testing.T) {
	d := Route("rename the variable", "claude-code", "")
	if d.Verdict != VerdictUnknown {
		t.Fatalf("verdict = %s, want UNKNOWN", d.Verdict)
	}
	// Still recommends the right tier.
	if d.Model.ID != "claude-haiku-4" {
		t.Errorf("recommended = %s, want claude-haiku-4", d.Model.ID)
	}
}

func TestRoute_Codex(t *testing.T) {
	// Trivial task on codex frontier → downshift to gpt-4o-mini.
	d := Route("fix a typo", "codex", "o3")
	if d.Verdict != VerdictDownshift {
		t.Fatalf("verdict = %s, want DOWNSHIFT", d.Verdict)
	}
	if d.Model.ID != "gpt-4o-mini" {
		t.Errorf("recommended = %s, want gpt-4o-mini", d.Model.ID)
	}
}

func TestSavingsRatio(t *testing.T) {
	opus := Catalog["claude-code"][TierFrontier]
	haiku := Catalog["claude-code"][TierSmall]

	got := SavingsRatio(opus, haiku)
	want := (90.0 - 4.8) / 90.0 // ~0.9467
	if math.Abs(got-want) > 0.001 {
		t.Errorf("SavingsRatio = %.4f, want %.4f", got, want)
	}

	// Not cheaper → 0
	if r := SavingsRatio(haiku, opus); r != 0 {
		t.Errorf("SavingsRatio(cheaper→pricier) = %.4f, want 0", r)
	}
}

func TestModelFor_UnknownHarness(t *testing.T) {
	// Unknown harness falls back to claude-code catalog.
	m := ModelFor("some-unknown-harness", TierSmall)
	if m.ID != "claude-haiku-4" {
		t.Errorf("fallback = %s, want claude-haiku-4", m.ID)
	}
}

func TestDecisionSummary(t *testing.T) {
	d := Route("rename the variable", "claude-code", "claude-opus-4-8")
	s := d.Summary()
	if s == "" {
		t.Error("Summary() should not be empty")
	}
	// Should mention downshift and the target model.
	if !contains(s, "downshift") || !contains(s, "claude-haiku-4") {
		t.Errorf("Summary() = %q, want downshift + target model", s)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
