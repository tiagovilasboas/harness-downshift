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
	"sort"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
)

// overrideCatalogFile is the path (relative to home) that pull writes.
const overrideCatalogFile = ".harness-downshift/catalog.json"

// pullFile is the JSON structure written to disk.
type pullFile struct {
	Doc     string          `json:"_doc"`
	Version string          `json:"version"`
	Updated string          `json:"updated"`
	Entries []catalog.Entry `json:"entries"`
}

// Pull discovers new models from provider APIs and merges them into
// ~/.harness-downshift/catalog.json. New models get tier=unknown for the
// user to assign. Existing entries keep their tier. Idempotent.
func Pull(cat CatalogReader, w, errW io.Writer) int {
	return PullWithProviders(cat, defaultProviders, &http.Client{Timeout: 10 * time.Second}, w, errW)
}

// PullWithProviders is the testable core of Pull. Tests inject mock providers
// pointing at httptest.Server URLs and a pre-configured HTTP client.
func PullWithProviders(cat CatalogReader, providers []ProviderConfig, client *http.Client, w, errW io.Writer) int {
	// Build lookup from existing entries so we don't overwrite existing tiers.
	existing := make(map[string]catalog.Entry)
	for _, e := range cat.Entries() {
		existing[entryKey(e.Harness, e.ID)] = e
	}

	added := 0
	for _, p := range providers {
		key := os.Getenv(p.EnvKey)
		if key == "" {
			fmt.Fprintf(w, "%-10s skipped (no %s)\n", p.Name, p.EnvKey)
			continue
		}
		ids, err := fetchModelIDs(client, p, key)
		if err != nil {
			fmt.Fprintf(errW, "%-10s fetch error: %v (skipping)\n", p.Name, err)
			continue
		}
		_, newModels := diffModels(cat, p.Harness, ids)
		for _, id := range newModels {
			k := entryKey(p.Harness, id)
			if _, exists := existing[k]; !exists {
				existing[k] = catalog.Entry{
					ID:      id,
					Harness: p.Harness,
					Tier:    "unknown",
				}
				added++
			}
		}
		fmt.Fprintf(w, "%-10s %d new models added\n", p.Name, len(newModels))
	}

	if added == 0 {
		fmt.Fprintln(w, "\nCatalog is up to date — no new models found.")
		return 0
	}

	// Flatten: preserve original order, append new entries at the end.
	var merged []catalog.Entry
	seen := make(map[string]bool)
	for _, e := range cat.Entries() {
		k := entryKey(e.Harness, e.ID)
		merged = append(merged, existing[k])
		seen[k] = true
	}
	var appended []catalog.Entry
	for k, e := range existing {
		if !seen[k] {
			appended = append(appended, e)
		}
	}
	sort.Slice(appended, func(i, j int) bool {
		if appended[i].Harness != appended[j].Harness {
			return appended[i].Harness < appended[j].Harness
		}
		return appended[i].ID < appended[j].ID
	})
	merged = append(merged, appended...)

	if err := writeOverride(merged); err != nil {
		fmt.Fprintf(errW, "error writing catalog: %v\n", err)
		return 1
	}

	home, _ := os.UserHomeDir()
	fmt.Fprintf(w, "\n%d new model(s) written to %s\n", added, filepath.Join(home, overrideCatalogFile))
	fmt.Fprintln(w, "Assign a tier (small/mid/frontier) to each 'unknown' entry before use.")
	return 0
}

// writeOverride serialises entries to ~/.harness-downshift/catalog.json.
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

// entryKey builds a unique deduplication key.
func entryKey(harness, id string) string {
	return harness + ":" + id
}
