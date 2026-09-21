// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core_test

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

var escalCat = catalog.Load()

// --- EscalationIntent.Tier() and Effort() ---

func TestEscalationIntent_TrivialTierAndEffort(t *testing.T) {
	if core.TrivialIntent.Tier() != core.TierSmall {
		t.Errorf("TrivialIntent.Tier() = %s, want small", core.TrivialIntent.Tier())
	}
	if core.TrivialIntent.Effort() != core.EffortLow {
		t.Errorf("TrivialIntent.Effort() = %s, want low", core.TrivialIntent.Effort())
	}
}

func TestEscalationIntent_NormalTierAndEffort(t *testing.T) {
	if core.NormalIntent.Tier() != core.TierMid {
		t.Errorf("NormalIntent.Tier() = %s, want mid", core.NormalIntent.Tier())
	}
	if core.NormalIntent.Effort() != core.EffortMid {
		t.Errorf("NormalIntent.Effort() = %s, want medium", core.NormalIntent.Effort())
	}
}

func TestEscalationIntent_ReviewTierAndEffort(t *testing.T) {
	if core.ReviewIntent.Tier() != core.TierFrontier {
		t.Errorf("ReviewIntent.Tier() = %s, want frontier", core.ReviewIntent.Tier())
	}
	if core.ReviewIntent.Effort() != core.EffortHigh {
		t.Errorf("ReviewIntent.Effort() = %s, want high", core.ReviewIntent.Effort())
	}
}

func TestEscalationIntent_PreservedEffort(t *testing.T) {
	if core.PreservedIntent.Effort() != core.EffortHigh {
		t.Errorf("PreservedIntent.Effort() = %s, want high", core.PreservedIntent.Effort())
	}
}

// --- Route produces correct Intent ---

func TestRoute_TrivialPrompt_TrivialIntent(t *testing.T) {
	d := core.Route("rename the userId variable", "claude-code", "", escalCat)
	if d.Intent != core.TrivialIntent {
		t.Errorf("trivial prompt intent = %s, want trivial", d.Intent)
	}
}

func TestRoute_ImplementationPrompt_NormalIntent(t *testing.T) {
	d := core.Route("implement the CSV export feature", "claude-code", "", escalCat)
	if d.Intent != core.NormalIntent {
		t.Errorf("implementation prompt intent = %s, want normal", d.Intent)
	}
}

func TestRoute_RefactorPrompt_NormalIntent(t *testing.T) {
	// Refactor is complex construction, not deep analysis — NormalIntent.
	d := core.Route("rearchitect the payment flow across services", "claude-code", "", escalCat)
	if d.Intent != core.NormalIntent {
		t.Errorf("rearchitect prompt intent = %s, want normal (construction)", d.Intent)
	}
}

func TestRoute_CodeReviewPrompt_ReviewIntent(t *testing.T) {
	d := core.Route("do a code review of the auth module", "claude-code", "", escalCat)
	if d.Intent != core.ReviewIntent {
		t.Errorf("code review prompt intent = %s, want review", d.Intent)
	}
}

func TestRoute_SecurityAuditPrompt_ReviewIntent(t *testing.T) {
	d := core.Route("do a security audit of the payment service", "claude-code", "", escalCat)
	if d.Intent != core.ReviewIntent {
		t.Errorf("security audit prompt intent = %s, want review", d.Intent)
	}
}

func TestRoute_RFCPrompt_ReviewIntent(t *testing.T) {
	d := core.Route("write an RFC for the new webhook architecture", "claude-code", "", escalCat)
	if d.Intent != core.ReviewIntent {
		t.Errorf("RFC prompt intent = %s, want review", d.Intent)
	}
}

func TestRoute_ExplicitAstra_PreservedIntent(t *testing.T) {
	// gpt-6-astra is explicit_only in the embedded catalog.
	d := core.Route("rename the variable", "codex", "gpt-6-astra", escalCat)
	if d.Intent != core.PreservedIntent {
		t.Errorf("explicit astra intent = %s, want preserved", d.Intent)
	}
}

func TestRoute_NormalModelNotPreserved(t *testing.T) {
	frontierID := escalCat.ModelFor("codex", core.TierFrontier).ID
	d := core.Route("rename the variable", "codex", frontierID, escalCat)
	if d.Intent == core.PreservedIntent {
		t.Errorf("normal frontier model should not produce PreservedIntent")
	}
}

// --- EscalationIntent.String() ---

func TestEscalationIntent_String(t *testing.T) {
	cases := map[core.EscalationIntent]string{
		core.TrivialIntent:   "trivial",
		core.NormalIntent:    "normal",
		core.ReviewIntent:    "review",
		core.PreservedIntent: "preserved",
	}
	for intent, want := range cases {
		if got := intent.String(); got != want {
			t.Errorf("%T(%d).String() = %q, want %q", intent, intent, got, want)
		}
	}
}

// --- Summary includes intent suffix for review/preserved ---

func TestDecisionSummary_ReviewIntentSuffix(t *testing.T) {
	d := core.Route("do a code review of the auth module", "claude-code", "", escalCat)
	s := d.Summary()
	if d.Intent == core.ReviewIntent {
		found := false
		for _, sub := range []string{"[review]", "review"} {
			if len(s) >= len(sub) {
				for i := 0; i <= len(s)-len(sub); i++ {
					if s[i:i+len(sub)] == sub {
						found = true
						break
					}
				}
			}
		}
		if !found {
			t.Errorf("Summary() for ReviewIntent missing 'review' suffix: %q", s)
		}
	}
}
