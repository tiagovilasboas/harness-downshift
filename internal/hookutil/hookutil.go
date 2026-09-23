// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package hookutil provides small utilities shared across harness adapters.
package hookutil

import "strings"

// StringField reads a string value from a decoded JSON object (map[string]any).
// Returns an empty string if the key is absent or the value is not a string.
func StringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// TaskText returns the first non-empty text field named in keys. Adapters use
// it to extract features while leaving persistence to the shared loop layer.
func TaskText(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(StringField(m, key)); value != "" {
			return value
		}
	}
	return ""
}
