// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1

package extractor

// contextSize estimates required context based on prompt length.
// Longer prompts usually describe more complex tasks needing more context.
func contextSize(words int) float64 {
	// Scale: 0-50 words → 0.1, 50-200 → 0.3-0.5, 200-500 → 0.6-0.8, 500+ → 0.9+
	switch {
	case words < 20:
		return 0.1
	case words < 50:
		return 0.2
	case words < 100:
		return 0.3
	case words < 200:
		return 0.5
	case words < 500:
		return 0.7
	case words < 1000:
		return 0.85
	default:
		return 0.95
	}
}
