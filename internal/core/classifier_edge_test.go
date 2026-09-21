// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

import "testing"

// TestClassify_EdgeCases covers prompts that are ambiguous, very short,
// mixed-signal, or phrased in ways that are easy to misclassify.
// Each entry is a documented expectation — if the classifier changes its
// mind on one of these, a test failure surfaces the regression immediately.
func TestClassify_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		want    Complexity
		note    string // why this classification is correct
	}{
		// --- Short prompts ---
		{
			name:  "single word rename",
			prompt: "rename",
			want:  Trivial,
			note:  "single strong trivial signal with short-prompt nudge",
		},
		{
			name:  "two words fix typo",
			prompt: "fix typo",
			want:  Trivial,
			note:  "trivial signal wins even without article",
		},
		{
			name:  "single word refactor",
			prompt: "refactor",
			want:  Medium,
			note:  "medium signal; short-prompt nudge alone can't make it trivial",
		},
		{
			name:  "one word prompt no signal",
			prompt: "help",
			want:  Medium,
			note:  "no content signal → safe Medium default",
		},

		// --- Mixed signals: tie must break toward higher complexity ---
		{
			name:  "rename inside migration",
			prompt: "rename the table as part of the migration",
			want:  Complex,
			note:  "rename (trivial 3) vs migration (complex 3) → tie → complex",
		},
		{
			name:  "format inside architecture",
			prompt: "format the output of the rearchitected pipeline",
			want:  Complex,
			note:  "format (trivial 3) vs rearchitect (complex 3) → complex wins",
		},
		{
			name:  "fix bug in distributed system",
			prompt: "fix the bug in the distributed payment processor",
			want:  Complex,
			note:  "fix bug (simple 2) vs distributed (complex 2) → tie → complex",
		},
		{
			name:  "implement with multi-tenant",
			prompt: "implement multi-tenant support across services",
			want:  Complex,
			note:  "implement (medium 2), multi-tenant (complex 3) → complex wins",
		},

		// --- Ambiguous real-world prompts ---
		{
			name:  "investigate performance",
			prompt: "investigate why the checkout is slow",
			want:  Medium,
			note:  "why+slow has no fail/crash/hang pattern — Medium is correct; only why+fail/crash triggers Complex",
		},
		{
			name:  "add logging to service",
			prompt: "add logging to the payment service",
			want:  Medium,
			note:  "add signal requires field/param/flag/method/function; 'logging' doesn't match — Medium default is correct",
		},
		{
			name:  "update a dependency",
			prompt: "update the lodash dependency to fix the vulnerability",
			want:  Medium,
			note:  "no strong trivial/complex signals → Medium default with some medium weight",
		},
		{
			name:  "write tests for module",
			prompt: "write tests for the auth module",
			want:  Simple,
			note:  "write test (simple 2) fires; module nudges medium but simple wins",
		},
		{
			name:  "run prettier on all files",
			prompt: "run prettier on all files in the src folder",
			want:  Trivial,
			note:  "prettier (trivial 3) is the dominant signal",
		},
		{
			name:  "explain the race condition",
			prompt: "explain the race condition in the payment observer",
			want:  Complex,
			note:  "race condition (complex 3) beats explain (simple 2)",
		},
		{
			name:  "design doc for new feature",
			prompt: "write a design doc for the new checkout flow",
			want:  Complex,
			note:  "design doc (complex 3) dominates write function (simple 2)",
		},
		{
			name:  "git push after refactor",
			prompt: "git push the refactor branch",
			want:  Trivial,
			note:  "git push (trivial 3) beats refactor (medium 2)",
		},

		// --- Long prompts nudge complex ---
		{
			name: "long vague prompt",
			prompt: "I need you to carefully review the entire codebase and " +
				"identify all the places where we might have issues with the " +
				"payment flow that could cause race conditions or deadlocks " +
				"especially in the distributed multi-tenant environment " +
				"and then suggest a comprehensive redesign with proper " +
				"tradeoffs and a design doc for the team to review",
			want: Complex,
			note: "multiple strong complex signals + long-prompt nudge",
		},

		// --- Confident flag ---
		{
			name:  "high confidence trivial",
			prompt: "rename the variable userID to userId in auth.ts",
			want:  Trivial,
			note:  "strong single signal → confident=true expected",
		},
		{
			name:  "low confidence vague",
			prompt: "look at this",
			want:  Medium,
			note:  "no content signal → not confident",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.prompt)
			if got.Complexity != tt.want {
				t.Errorf("Classify(%q) = %s, want %s (scores: %v)\nnote: %s",
					tt.prompt, got.Complexity, tt.want, got.Scores, tt.note)
			}
		})
	}
}

// TestClassify_ConfidenceFlag verifies that the Confident flag is set only
// when the top class clearly beat the runner-up (margin ≥ 2).
func TestClassify_ConfidenceFlag(t *testing.T) {
	strong := Classify("rename the variable userId") // rename=3, short nudge=1 → trivial 4 vs rest 0
	if !strong.Confident {
		t.Errorf("strong single-signal prompt should be confident; scores=%v", strong.Scores)
	}

	weak := Classify("help me with this thing") // no signal → Medium, not confident
	if weak.Confident {
		t.Errorf("no-signal prompt should not be confident; scores=%v", weak.Scores)
	}
}

// TestClassify_ScoresAllClasses verifies that the Scores map always has an
// entry for every complexity class — even when score is zero.
func TestClassify_ScoresAllClasses(t *testing.T) {
	got := Classify("rename the variable")
	for _, c := range []Complexity{Trivial, Simple, Medium, Complex} {
		if _, ok := got.Scores[c]; !ok {
			t.Errorf("Scores missing class %s", c)
		}
	}
}
