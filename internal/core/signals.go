// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

// SignalDef is one uncompiled classifier signal: a regex pattern, a weight,
// and the complexity class it votes for. Keeping the table here — separate
// from the classifier logic in classifier.go — allows tests to inspect,
// validate, and extend the signal set without touching the scoring engine.
type SignalDef struct {
	Pattern string
	Weight  int
	Class   Complexity
}

// RawSignals is the canonical, ordered list of classifier signals.
// The classifier compiles these at startup via compileSignals().
//
// To add or tune a signal: edit this slice and add a table-driven test case
// in classifier_edge_test.go that proves the change.
//
// Order matters only for readability — the scorer tries every signal
// independently and sums weights per class.
var RawSignals = []SignalDef{

	// ── COMPLEX — architecture, cross-system, security, migrations ────────

	{`\barchitect\w*\b`, 3, Complex},
	{`\bre-?architect\w*\b`, 3, Complex},
	{`\bmigrat\w*\b`, 3, Complex},
	{`\bre-?write\b`, 2, Complex},
	{`\bre-?design\b`, 3, Complex},
	{`\bsecurity\s+(audit|review)\b`, 3, Complex},
	{`\bcode\s+review\b`, 3, Complex},
	{`\bcross-system\b`, 3, Complex},
	{`\bdistributed\b`, 2, Complex},
	{`\brace\s+condition\b`, 3, Complex},
	{`\bdeadlock\b`, 3, Complex},
	{`\bmulti-(system|service|region|tenant)\b`, 3, Complex},
	{`\bentire\s+(codebase|system|platform)\b`, 3, Complex},
	{`\btrade-?offs?\b`, 2, Complex},
	{`\bdesign\s+doc\b`, 3, Complex},
	{`\brfc\b`, 3, Complex},
	{`\bthink\s+through\b`, 2, Complex},
	{`\bwhy\s+.{0,40}(fail|crash|hang|break|regress)\w*`, 2, Complex},

	// ── MEDIUM — features across files, refactors with context ───────────

	{`\brefactor\w*\b`, 2, Medium},
	{`\bimplement\b`, 2, Medium},
	{`\bintegrat\w*\b`, 2, Medium},
	{`\bfeature\b`, 2, Medium},
	{`\bendpoint\b`, 1, Medium},
	{`\bmodule\b`, 1, Medium},
	{`\bacross\s+\w+\s+files\b`, 2, Medium},
	{`\bdebug\b`, 2, Medium},
	{`\brevis(?:ar|e|ão|oes|ões)\b`, 2, Medium},

	// ── SIMPLE — one isolated change ─────────────────────────────────────

	{`\badd\s+(a\s+)?(field|param|flag|method|function)\b`, 2, Simple},
	{`\bfix\s+(the\s+)?bug\b`, 2, Simple},
	{`\bwrite\s+(a\s+)?(function|tests?\b|helper)\b`, 2, Simple},
	{`\bexplain\b`, 2, Simple},
	{`\bwhat\s+(is|does|are)\b`, 1, Simple},
	{`\bsingle\s+(file|function)\b`, 2, Simple},

	// ── TRIVIAL — mechanical ──────────────────────────────────────────────

	{`\brename\b`, 3, Trivial},
	{`\brenome\w*\b`, 3, Trivial},
	{`\bformat\b`, 3, Trivial},
	{`\bindent\b`, 3, Trivial},
	{`\blint\b`, 2, Trivial},
	{`\bprettier\b`, 3, Trivial},
	{`\beslint\b`, 3, Trivial},
	{`\bgit\s+(commit|push|pull|status|add|stash|log|diff|checkout|branch)\b`, 3, Trivial},
	{`\bfix\s+(a\s+)?typo\b`, 3, Trivial},
	{`\badd\s+(a\s+)?comment\b`, 3, Trivial},
	{`\bboilerplate\b`, 2, Trivial},
	{`\bmove\s+(the\s+)?file\b`, 3, Trivial},
	{`\bdelete\s+(the\s+)?file\b`, 3, Trivial},
	{`\bbump\s+.{0,15}version\b`, 3, Trivial},
	{`\bremove\s+(unused|dead)\b`, 2, Trivial},
	{`\blist(?:ar|e)?\b.{0,40}\barquivos?\b`, 3, Trivial},
}
