// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
)

func TestWriteOverride_AtomicallyWritesPrivateCatalog(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	entries := []catalog.Entry{{ID: "test-model", Harness: "codex", Tier: "small"}}

	if err := writeOverride(entries); err != nil {
		t.Fatalf("writeOverride() error: %v", err)
	}

	dir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir(): %v", err)
	}
	dir = filepath.Join(dir, ".harness-downshift")
	path := filepath.Join(dir, "catalog.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q): %v", path, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("catalog permissions = %o, want 600", got)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	if len(files) != 1 || files[0].Name() != "catalog.json" {
		t.Errorf("temporary catalog files left behind: %#v", files)
	}
}
