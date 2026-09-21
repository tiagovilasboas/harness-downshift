// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package models

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// List prints the effective catalog as a formatted table to w.
// Shows harness, tier, model ID, cost per 1M tokens, effort scale, and
// which source is active (embedded or user override).
func List(cat CatalogReader, w io.Writer) {
	entries := cat.Entries()
	if len(entries) == 0 {
		fmt.Fprintln(w, "catalog is empty")
		return
	}

	// Sort: harness → tier order (small, mid, frontier, unknown).
	tierOrder := map[string]int{"small": 0, "mid": 1, "frontier": 2, "unknown": 3}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Harness != entries[j].Harness {
			return entries[i].Harness < entries[j].Harness
		}
		oi := tierOrder[strings.ToLower(entries[i].Tier)]
		oj := tierOrder[strings.ToLower(entries[j].Tier)]
		return oi < oj
	})

	fmt.Fprintln(w, "HARNESS        TIER       MODEL                  IN $/1M  OUT $/1M  EFFORT SCALE")
	fmt.Fprintln(w, "─────────────  ─────────  ─────────────────────  ───────  ────────  ────────────")

	for _, e := range entries {
		// Skip section comment entries (id or harness is empty).
		if e.ID == "" || e.Harness == "" {
			continue
		}
		fmt.Fprintf(w, "%-13s  %-9s  %-21s  %7.2f  %8.2f  %s\n",
			e.Harness,
			e.Tier,
			e.ID,
			e.InputCostM,
			e.OutputCostM,
			e.EffortScale,
		)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Catalog source: %s\n", catalogSource())
}

// catalogSource returns a human-readable description of which catalog is active.
func catalogSource() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "embedded (default)"
	}
	override := filepath.Join(home, ".harness-downshift", "catalog.json")
	if _, err := os.Stat(override); err == nil {
		return fmt.Sprintf("user override (%s)", override)
	}
	return "embedded (default) — override at ~/.harness-downshift/catalog.json"
}
