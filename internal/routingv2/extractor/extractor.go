// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

// Package extractor extracts normalized feature signals from a task prompt.
// All signals are in [0, 1]. The same extraction logic is used at runtime
// and during offline training to ensure consistency.
package extractor

import "github.com/tiagovilasboas/harness-downshift/internal/routingv2/domain"

// Extract analyzes the prompt and returns a FeatureVector with 13 signals.
// Each signal is normalized to [0, 1].
func Extract(prompt string) domain.FeatureVector {
	lower := toLower(prompt)
	words := wordCount(prompt)

	return domain.FeatureVector{
		Mechanical:   mechanical(lower),
		Coding:       coding(lower),
		Debugging:    debugging(lower),
		Refactoring:  refactoring(lower),
		Architecture: architecture(lower),
		Migration:    migration(lower),
		Security:     security(lower),
		Concurrency:  concurrency(lower),
		Planning:     planning(lower),
		ToolUse:      toolUse(lower),
		Ambiguity:    ambiguity(lower),
		CrossModule:  crossModule(lower),
		ContextSize:  contextSize(words),
	}
}
