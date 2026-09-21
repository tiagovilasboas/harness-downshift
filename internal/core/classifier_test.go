// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package core

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   Complexity
	}{
		// Trivial — mechanical
		{"rename", "rename the variable userID to userId", Trivial},
		{"fix typo", "fix a typo in the README", Trivial},
		{"format", "format this file with prettier", Trivial},
		{"git commit", "git commit all the changes and push", Trivial},
		{"add comment", "add a comment explaining this loop", Trivial},
		{"bump version", "bump the version to 2.1.0", Trivial},
		{"portuguese list files", "liste apenas os arquivos .go", Trivial},

		// Simple — one isolated change
		{"add field", "add a field email to the User struct", Simple},
		{"fix bug", "fix the bug where login fails on empty password", Simple},
		{"write function", "write a function that validates a CPF", Simple},
		{"explain", "explain what this regex does", Simple},

		// Medium — feature across files
		{"refactor", "refactor the auth module to use the new client", Medium},
		{"implement feature", "implement the feature to export sales as CSV", Medium},
		{"debug context", "debug why the webhook handler drops events", Medium},

		// Complex — architecture, cross-system
		{"architecture", "rearchitect the payment flow across services", Complex},
		{"migration", "plan the migration from v4 to v5 gateway", Complex},
		{"security audit", "do a security audit of the auth layer", Complex},
		{"race condition", "there is a race condition in the observer, find it", Complex},
		{"redesign", "redesign the checkout to support multi-tenant", Complex},
		{"code review", "list the Go files and do a code review without changing anything", Complex},

		// Default — no signal
		{"empty", "", Medium},
		{"vague", "help me with this", Medium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.prompt)
			if got.Complexity != tt.want {
				t.Errorf("Classify(%q) = %s, want %s (scores: %v)",
					tt.prompt, got.Complexity, tt.want, got.Scores)
			}
		})
	}
}

func TestClassify_TieBreaksToHigher(t *testing.T) {
	// A prompt with both trivial and complex signals must not underpower.
	// "rename" (trivial 3) vs "migration" (complex 3) — tie should favor complex.
	got := Classify("rename things as part of the migration")
	if got.Complexity != Complex {
		t.Errorf("tie should favor higher complexity, got %s (scores %v)",
			got.Complexity, got.Scores)
	}
}

func TestComplexityTier(t *testing.T) {
	tests := []struct {
		c    Complexity
		tier Tier
	}{
		{Trivial, TierSmall},
		{Simple, TierMid},
		{Medium, TierMid},
		{Complex, TierFrontier},
	}
	for _, tt := range tests {
		if got := tt.c.Tier(); got != tt.tier {
			t.Errorf("%s.Tier() = %s, want %s", tt.c, got, tt.tier)
		}
	}
}

func TestComplexityString(t *testing.T) {
	cases := map[Complexity]string{
		Trivial: "TRIVIAL", Simple: "SIMPLE", Medium: "MEDIUM", Complex: "COMPLEX",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("String() = %s, want %s", got, want)
		}
	}
}
