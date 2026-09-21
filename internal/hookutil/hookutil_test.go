// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package hookutil_test

import (
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/hookutil"
)

func TestStringField(t *testing.T) {
	tests := []struct {
		name  string
		m     map[string]any
		key   string
		want  string
	}{
		{
			name: "present string value",
			m:    map[string]any{"model": "claude-haiku-4"},
			key:  "model",
			want: "claude-haiku-4",
		},
		{
			name: "absent key returns empty",
			m:    map[string]any{"model": "claude-haiku-4"},
			key:  "prompt",
			want: "",
		},
		{
			name: "nil map returns empty",
			m:    nil,
			key:  "model",
			want: "",
		},
		{
			name: "empty map returns empty",
			m:    map[string]any{},
			key:  "model",
			want: "",
		},
		{
			name: "value is int not string",
			m:    map[string]any{"count": 42},
			key:  "count",
			want: "",
		},
		{
			name: "value is bool not string",
			m:    map[string]any{"flag": true},
			key:  "flag",
			want: "",
		},
		{
			name: "value is nil",
			m:    map[string]any{"model": nil},
			key:  "model",
			want: "",
		},
		{
			name: "value is nested map not string",
			m:    map[string]any{"nested": map[string]any{"a": "b"}},
			key:  "nested",
			want: "",
		},
		{
			name: "empty string value",
			m:    map[string]any{"model": ""},
			key:  "model",
			want: "",
		},
		{
			name: "key with spaces",
			m:    map[string]any{"tool name": "Task"},
			key:  "tool name",
			want: "Task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hookutil.StringField(tt.m, tt.key)
			if got != tt.want {
				t.Errorf("StringField(%v, %q) = %q, want %q", tt.m, tt.key, got, tt.want)
			}
		})
	}
}
