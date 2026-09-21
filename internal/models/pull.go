// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
)

// overrideCatalogFile is the file that pull writes/updates.
const overrideCatalogFile = ".harness-downshift/catalog.json"

// pullFile is the JSON structure written by pull.
type pullFile struct {
	Doc     string          `json:"_doc"`
	Version string          `json:"version"`
	Updated string          `json:"updated"`
	Entries []catalog.Entry `json:"entries"`
}

// Pull discovers new models from provider APIs and merges them into the user's
// catalog override at ~/.harness-downshift/catalog.json.
//
// Rules:
//   - Existing entries keep their tier — never overwritten.
//   - New models are added with tier="unknown" for the user to assign.
//   - The operation is idempotent: running twice produces the same result.
//   - Missing API keys skip that provider.
//   - Any network/parse error skips that provider (fail-open).
//
// Returns exit code 0 on success, 1 on a fatal file-system error.
func Pull(cat *catalog.Catalog, w, errW io.Writer) int {
	client := &http.Client{Timeout: 10 * time.Second}

	// Start from the current entries (embedded + any existing override).
	existing := make(map[string]catalog.Entry) // key: harness+":"+id
	for _, e := range cat.Entries() {
		existing[entryKey(e.Harness, e.ID)] = e
	}

	added := 0
	for _, p := range providers {
		key := os.Getenv(p.envKey)
		if key == "" {
			fmt.Fprintf(w, "%-10s skipped (no %s)\n", p.name, p.envKey)
			continue
		}
		ids, err := fetchModelIDs(client, p, key)
		if err != nil {
			fmt.Fprintf(errW, "%-10s fetch error: %v (skipping)\n", p.name, err)
			continue
		}
		_, newModels := diffModels(cat, p.harness, ids)
		for _, id := range newModels {
			k := entryKey(p.harness, id)
			if _, exists := existing[k]; !exists {
				existing[k] = catalog.Entry{
					ID:      id,
					Harness: p.harness,
					Tier:    "unknown",
				}
				added++
			}
		}
		fmt.Fprintf(w, "%-10s %d new models added\n", p.name, len(newModels))
	}

	if added == 0 {
		fmt.Fprintln(w, "\nCatalog is up to date — no new models found.")
		return 0
	}

	// Flatten map back to slice, preserving insertion order from existing catalog.
	var merged []catalog.Entry
	seen := make(map[string]bool)
	for _, e := range cat.Entries() {
		k := entryKey(e.Harness, e.ID)
		merged = append(merged, existing[k])
		seen[k] = true
	}
	for k, e := range existing {
		if !seen[k] {
			merged = append(merged, e)
		}
	}

	if err := writeOverride(merged); err != nil {
		fmt.Fprintf(errW, "error writing catalog: %v\n", err)
		return 1
	}

	home, _ := os.UserHomeDir()
	path := filepath.Join(home, overrideCatalogFile)
	fmt.Fprintf(w, "\n%d new model(s) written to %s\n", added, path)
	fmt.Fprintln(w, "Assign a tier (small/mid/frontier) to each 'unknown' entry before use.")
	return 0
}

// writeOverride serialises entries to ~/.harness-downshift/catalog.json,
// creating the directory if needed.
func writeOverride(entries []catalog.Entry) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".harness-downshift")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f := pullFile{
		Doc:     "harness-downshift user catalog — edit tier/effort fields and reload.",
		Version: "1",
		Updated: time.Now().UTC().Format("2006-01-02"),
		Entries: entries,
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "catalog.json"), data, 0o600)
}

// entryKey builds a unique key for deduplication.
func entryKey(harness, id string) string {
	return harness + ":" + id
}
