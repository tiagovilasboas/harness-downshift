// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

import (
	"regexp"
	"strings"
)

// Complexity is the difficulty class of a task. It maps directly to a Tier.
type Complexity int

const (
	// Trivial — mechanical work: rename, format, move, git ops, boilerplate.
	Trivial Complexity = iota
	// Simple — one isolated change: a field, a function, a small bug fix.
	Simple
	// Medium — a feature across a few files, a refactor with context.
	Medium
	// Complex — architecture, cross-system debugging, security, migrations.
	Complex
)

// String returns the human-readable complexity name.
func (c Complexity) String() string {
	switch c {
	case Trivial:
		return "TRIVIAL"
	case Simple:
		return "SIMPLE"
	case Medium:
		return "MEDIUM"
	case Complex:
		return "COMPLEX"
	default:
		return "UNKNOWN"
	}
}

// signal is a weighted matcher for a complexity class.
type signal struct {
	re     *regexp.Regexp
	weight int
	class  Complexity
}

// signals are ordered from strongest to weakest evidence. We score every
// class and pick the top; ties break toward the higher complexity to stay
// safe (never underpower a task).
var signals = compileSignals()

func compileSignals() []signal {
	out := make([]signal, 0, len(RawSignals))
	for _, r := range RawSignals {
		out = append(out, signal{
			re:     regexp.MustCompile(`(?i)` + r.Pattern),
			weight: r.Weight,
			class:  r.Class,
		})
	}
	return out
}

// Classification is the result of scoring a task prompt.
type Classification struct {
	Complexity Complexity
	Scores     map[Complexity]int
	Confident  bool // true when the top class clearly beat the rest
}

// Classify scores a task prompt against all complexity classes and returns
// the winning complexity. Deterministic, no LLM call, no network.
//
// Rules:
//   - Sum weights per class from matching signals.
//   - Pick the highest-scoring class.
//   - Tie breaks toward higher complexity (never underpower a task).
//   - No signal at all => Medium (safe default, the harness's usual model).
func Classify(prompt string) Classification {
	lower := strings.ToLower(prompt)
	scores := map[Complexity]int{Trivial: 0, Simple: 0, Medium: 0, Complex: 0}

	for _, s := range signals {
		if s.re.MatchString(lower) {
			scores[s.class] += s.weight
		}
	}

	// Track whether any content signal fired. Structural nudges alone must not
	// decide the class — a vague prompt with no real signal stays Medium.
	contentMatched := scores[Trivial]+scores[Simple]+scores[Medium]+scores[Complex] > 0

	// Structural nudge: very short imperative prompts lean trivial — but only
	// to reinforce an existing trivial signal, never to create one.
	wordCount := len(strings.Fields(prompt))
	if contentMatched && wordCount > 0 && wordCount <= 6 {
		scores[Trivial]++
	}
	// Long, detailed prompts lean toward higher complexity.
	if contentMatched && wordCount >= 80 {
		scores[Complex]++
	}

	// No content signal at all => safe default (the harness's usual model).
	if !contentMatched {
		return Classification{Complexity: Medium, Scores: scores, Confident: false}
	}

	// Pick the top score. Iterate high→low so ties favor higher complexity.
	order := []Complexity{Complex, Medium, Simple, Trivial}
	top := Medium // safe default when nothing matches
	topScore := 0
	for _, c := range order {
		if scores[c] > topScore {
			topScore = scores[c]
			top = c
		}
	}

	// Confidence: the top class beat the runner-up by a clear margin.
	second := 0
	for _, c := range order {
		if c == top {
			continue
		}
		if scores[c] > second {
			second = scores[c]
		}
	}
	confident := topScore > 0 && topScore-second >= 2

	return Classification{
		Complexity: top,
		Scores:     scores,
		Confident:  confident,
	}
}
