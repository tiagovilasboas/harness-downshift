// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// coverage_test.go covers low-traffic paths not reached by the main test suite.
package core_test

import (
	"strings"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

var covCat = catalog.Load()

// --- Verdict.String() ---

func TestVerdictString(t *testing.T) {
	cases := map[core.Verdict]string{
		core.VerdictOK:        "OK",
		core.VerdictDownshift: "DOWNSHIFT",
		core.VerdictUpshift:   "UPSHIFT",
		core.VerdictUnknown:   "UNKNOWN",
		core.Verdict(99):      "UNKNOWN", // default branch
	}
	for v, want := range cases {
		if got := v.String(); got != want {
			t.Errorf("Verdict(%d).String() = %q, want %q", v, got, want)
		}
	}
}

// --- Effort.String() default branch ---

func TestEffortString_DefaultBranch(t *testing.T) {
	// Effort(99) hits the default case → "medium"
	if got := core.Effort(99).String(); got != "medium" {
		t.Errorf("Effort(99).String() = %q, want medium", got)
	}
}

// --- EscalationIntent.String() default branch ---

func TestEscalationIntentString_DefaultBranch(t *testing.T) {
	if got := core.EscalationIntent(99).String(); got != "unknown" {
		t.Errorf("EscalationIntent(99).String() = %q, want unknown", got)
	}
}

// --- EscalationIntent.Tier() default / unknown branch ---

func TestEscalationIntentTier_DefaultBranch(t *testing.T) {
	// Anything other than Trivial/Review/Preserved → TierMid
	if got := core.EscalationIntent(99).Tier(); got != core.TierMid {
		t.Errorf("EscalationIntent(99).Tier() = %s, want mid", got)
	}
}

// --- Plan() with PreserveExplicit ---

func TestPlan_PreserveExplicit_SkipsRewrite(t *testing.T) {
	// Route a trivial task on an explicit_only model (gpt-6-astra).
	d := core.Route("rename the variable", "codex", "gpt-6-astra", covCat)
	plan := d.Plan(core.CodexCaps, covCat)
	if !plan.PreserveExplicit {
		t.Error("Plan must set PreserveExplicit for explicit_only current model")
	}
	if plan.RewriteModel {
		t.Error("Plan must not rewrite model when PreserveExplicit")
	}
	if plan.ApplyEffort {
		t.Error("Plan must not apply effort when PreserveExplicit")
	}
}

func TestPlan_NilResolver_DoesNotPreserve(t *testing.T) {
	// Without a resolver, PreserveExplicit must be false (fail-open).
	d := core.Route("rename the variable", "codex", "gpt-6-astra")
	plan := d.Plan(core.CodexCaps) // no resolver
	if plan.PreserveExplicit {
		t.Error("Plan without resolver must not set PreserveExplicit")
	}
}

// --- ShouldRewriteModel — empty model ID ---

func TestShouldRewriteModel_EmptyModelID(t *testing.T) {
	// A Decision with no recommended model must never trigger a rewrite.
	d := core.Decision{}
	if d.ShouldRewriteModel() {
		t.Error("empty model ID must return false from ShouldRewriteModel")
	}
}

// --- resolveModel with no resolver (nil path) ---

func TestRoute_NilResolver_ReturnsEmptyModelID(t *testing.T) {
	// Route without a resolver cannot look up a model — returns empty ID.
	// This is intentional: callers must inject a catalog resolver.
	d := core.Route("rename the variable", "claude-code", "")
	if d.Model.ID != "" {
		t.Errorf("Route with nil resolver should return empty model ID, got %q", d.Model.ID)
	}
	// Complexity classification still works without a resolver.
	if d.Complexity != core.Trivial {
		t.Errorf("complexity = %s, want TRIVIAL", d.Complexity)
	}
	// Verdict must be Unknown (no current model, no recommended model).
	if d.Verdict != core.VerdictUnknown {
		t.Errorf("verdict = %s, want UNKNOWN", d.Verdict)
	}
}

// --- Summary: PreservedIntent suffix ---

func TestDecisionSummary_PreservedIntentSuffix(t *testing.T) {
	d := core.Route("rename the variable", "codex", "gpt-6-astra", covCat)
	s := d.Summary()
	if !strings.Contains(s, "preserved") {
		t.Errorf("Summary for PreservedIntent should contain 'preserved'; got %q", s)
	}
}

// --- isReviewTask via Route for each review signal ---

func TestReviewSignals_AllTriggerReviewIntent(t *testing.T) {
	prompts := []string{
		"do a code review of the auth module",
		"perform a security audit of the payment service",
		"run a security review on the webhook handler",
		"write an rfc for the new checkout architecture",
		"create a design doc for the notification system",
		"think through the tradeoffs of the new caching layer",
	}
	for _, prompt := range prompts {
		d := core.Route(prompt, "claude-code", "", covCat)
		if d.Intent != core.ReviewIntent {
			t.Errorf("prompt %q → intent %s, want review", prompt, d.Intent)
		}
	}
}

// --- contains edge cases ---

func TestContainsEdge_SubstrLongerThanString(t *testing.T) {
	// isReviewTask calls contains internally; exercise the len guard via Route
	// with a very short prompt that can't match any multi-word review signal.
	d := core.Route("rfc", "claude-code", "", covCat)
	// "rfc" matches the rfc review signal
	if d.Intent != core.ReviewIntent {
		t.Errorf("single-word 'rfc' prompt should be ReviewIntent, got %s", d.Intent)
	}
}
