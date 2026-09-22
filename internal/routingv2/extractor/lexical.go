// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package extractor

import (
	"strings"
	"unicode"
)

// toLower normalizes prompt to lowercase for matching.
func toLower(s string) string {
	return strings.ToLower(s)
}

// wordCount returns approximate word count.
func wordCount(s string) int {
	return len(strings.Fields(s))
}

// hasAny returns true if text contains any of the keywords.
func hasAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// countMatches returns how many keywords appear in text.
func countMatches(text string, keywords ...string) int {
	count := 0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			count++
		}
	}
	return count
}

// normalize clamps x to [0, 1].
func normalize(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// mechanical detects rename, format, lint, simple fixes.
func mechanical(text string) float64 {
	keywords := []string{
		"rename", "format", "lint", "typo", "spelling",
		"whitespace", "indent", "import", "unused",
		"add comment", "remove comment", "fix typo",
		"update version", "bump version",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.4)
}

// coding detects code production tasks.
func coding(text string) float64 {
	keywords := []string{
		"implement", "create", "add", "write", "build",
		"function", "method", "class", "component", "endpoint",
		"api", "handler", "service", "module", "feature",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.15)
}

// debugging detects bug investigation.
func debugging(text string) float64 {
	keywords := []string{
		"bug", "fix", "error", "issue", "crash", "fail",
		"broken", "wrong", "incorrect", "debug", "investigate",
		"trace", "stacktrace", "exception", "null", "undefined",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.2)
}

// refactoring detects code restructuring.
func refactoring(text string) float64 {
	keywords := []string{
		"refactor", "restructure", "reorganize", "clean up",
		"extract", "inline", "move", "split", "merge",
		"simplify", "deduplicate", "dry",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.25)
}

// architecture detects system design tasks.
func architecture(text string) float64 {
	keywords := []string{
		"architect", "design", "pattern", "structure",
		"layer", "module", "component", "system",
		"scalab", "extensib", "maintain", "decoupl",
		"interface", "abstract", "dependency",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.2)
}

// migration detects data/schema migration.
func migration(text string) float64 {
	keywords := []string{
		"migrat", "schema", "database", "table", "column",
		"alter", "drop", "data transfer", "etl", "transform",
		"upgrade", "downgrade", "backward compat",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.3)
}

// security detects auth, crypto, access control.
func security(text string) float64 {
	keywords := []string{
		"auth", "login", "password", "token", "jwt", "oauth",
		"permission", "role", "access", "secret", "encrypt",
		"decrypt", "hash", "ssl", "tls", "certificate",
		"vulnerability", "injection", "xss", "csrf", "sanitiz",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.2)
}

// concurrency detects race conditions, deadlocks.
func concurrency(text string) float64 {
	keywords := []string{
		"concurrent", "parallel", "thread", "goroutine",
		"mutex", "lock", "deadlock", "race", "atomic",
		"channel", "async", "await", "promise", "future",
		"synchron", "semaphore",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.25)
}

// planning detects multi-step decomposition.
func planning(text string) float64 {
	keywords := []string{
		"plan", "step", "phase", "stage", "sequence",
		"first", "then", "after", "before", "finally",
		"breakdown", "decompos", "orchestrat",
	}
	matches := countMatches(text, keywords...)

	// Also boost if prompt has numbered list
	if hasNumberedList(text) {
		matches += 2
	}

	return normalize(float64(matches) * 0.2)
}

// toolUse detects external tool usage.
func toolUse(text string) float64 {
	keywords := []string{
		"run", "execute", "command", "script", "terminal",
		"shell", "bash", "npm", "yarn", "pip", "go build",
		"docker", "kubernetes", "deploy", "ci", "cd",
		"test", "lint", "build",
	}
	matches := countMatches(text, keywords...)
	return normalize(float64(matches) * 0.15)
}

// ambiguity detects vague or conflicting requirements.
func ambiguity(text string) float64 {
	keywords := []string{
		"maybe", "perhaps", "might", "could", "should",
		"not sure", "unclear", "depends", "either", "or",
		"something like", "kind of", "sort of",
		"?", "somehow", "whatever",
	}
	matches := countMatches(text, keywords...)

	// Very short prompts are often ambiguous
	if len(text) < 50 {
		matches += 2
	}

	return normalize(float64(matches) * 0.2)
}

// crossModule detects multi-module changes.
func crossModule(text string) float64 {
	keywords := []string{
		"multiple file", "several file", "across", "all",
		"everywhere", "global", "codebase", "project-wide",
		"repository", "monorepo", "package", "module",
	}
	matches := countMatches(text, keywords...)

	// Count path separators as hints
	slashes := strings.Count(text, "/")
	if slashes > 3 {
		matches += 1
	}

	return normalize(float64(matches) * 0.25)
}

// hasNumberedList checks for patterns like "1.", "2.", etc.
func hasNumberedList(text string) bool {
	for i, r := range text {
		if unicode.IsDigit(r) && i+1 < len(text) && text[i+1] == '.' {
			return true
		}
	}
	return false
}
