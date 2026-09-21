// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package catalog owns all model-ID data for harness-downshift.
//
// core never imports catalog. The dependency runs one way:
//
//	catalog → core (for Tier, Effort types)
//	adapters → catalog (for a Resolver at startup)
//
// Model data lives in catalog.json (embedded at compile time). Users may
// override it by placing a file at ~/.harness-downshift/catalog.json — the
// override is merged at load time, with the user file taking precedence.
// Any error reading the user file is logged to stderr and silently ignored;
// the embedded catalog is always the safe fallback.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "embed"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

//go:embed catalog.json
var embeddedJSON []byte

// Entry is one row in the catalog JSON.
type Entry struct {
	ID          string            `json:"id"`
	Aliases     []string          `json:"aliases"`
	Family      string            `json:"family"` // stable prefix for version-agnostic matching
	Provider    string            `json:"provider"`
	Harness     string            `json:"harness"`
	Tier        string            `json:"tier"`    // "small" | "mid" | "frontier" | "unknown"
	Routing     string            `json:"routing"` // "automatic" (default) | "explicit_only"
	EffortScale string            `json:"effort_scale"`
	EffortMap   map[string]string `json:"effort_map"` // "low"|"medium"|"high" → native string
	InputCostM  float64           `json:"input_cost_per_1m"`
	OutputCostM float64           `json:"output_cost_per_1m"`
}

// catalogFile is the structure of the JSON file on disk.
type catalogFile struct {
	Version string  `json:"version"`
	Updated string  `json:"updated"`
	Entries []Entry `json:"entries"`
}

// Catalog is the resolved in-memory model store, implementing core.Resolver.
type Catalog struct {
	// index: harness → tier → Model (primary lookup for routing)
	index map[string]map[core.Tier]core.Model
	// harnesses records every harness declared by the catalog, including one
	// whose current entries do not provide every routable tier. A declared
	// harness must never borrow a model ID from another harness.
	harnesses map[string]struct{}
	// byID: harness → modelID/alias → Model (exact and alias lookup)
	byID map[string]map[string]core.Model
	// byFamily: harness → family prefix → Model (version-agnostic fallback)
	byFamily map[string][]familyEntry
	// entries: all raw entries (for list/check/pull commands)
	entries []Entry
}

// familyEntry pairs a stable family prefix with the model it maps to.
type familyEntry struct {
	prefix string
	model  core.Model
}

// Load returns the effective Catalog. Load priority:
//  1. ~/.harness-downshift/catalog.json (user override, if present and valid)
//  2. Embedded catalog.json (compile-time default, always valid)
//
// If the user file exists but cannot be parsed, a warning is printed to
// stderr and Load falls back to the embedded file — fail-open, always.
func Load() *Catalog {
	return load(os.Stderr)
}

// load resolves the embedded catalog and optional user override. warning is
// injected for tests; production callers use stderr through Load.
func load(warning io.Writer) *Catalog {
	base, err := decode(embeddedJSON)
	if err != nil {
		panic("harness-downshift: embedded catalog.json is invalid: " + err.Error())
	}

	// A user catalog is an override, not a replacement: preserve all embedded
	// entries that it does not name and replace only matching harness/model IDs.
	if path, ok := userCatalogPath(); ok {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(warning, "downshift: warning: could not read %s (%v); using embedded catalog\n", path, err)
		} else if override, err := decode(data); err == nil {
			merged := mergeEntries(base.Entries, override.Entries)
			if err := validateEntries(merged); err != nil {
				fmt.Fprintf(warning, "downshift: warning: could not merge %s (%v); using embedded catalog\n", path, err)
			} else {
				base.Entries = merged
			}
		} else {
			fmt.Fprintf(warning, "downshift: warning: could not parse %s (%v); using embedded catalog\n", path, err)
		}
	}
	return build(base)
}

// userCatalogPath returns the path to the user override file and whether it
// exists. Returns false if XDG / home resolution fails.
func userCatalogPath() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	p := filepath.Join(home, ".harness-downshift", "catalog.json")
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}

// parse decodes a catalog JSON payload into a Catalog.
func parse(data []byte) (*Catalog, error) {
	f, err := decode(data)
	if err != nil {
		return nil, err
	}
	return build(f), nil
}

func decode(data []byte) (catalogFile, error) {
	var f catalogFile
	if err := json.Unmarshal(data, &f); err != nil {
		return catalogFile{}, err
	}
	if err := validateEntries(f.Entries); err != nil {
		return catalogFile{}, err
	}
	return f, nil
}

func build(f catalogFile) *Catalog {
	c := &Catalog{
		index:     make(map[string]map[core.Tier]core.Model),
		harnesses: make(map[string]struct{}),
		byID:      make(map[string]map[string]core.Model),
		byFamily:  make(map[string][]familyEntry),
		entries:   f.Entries,
	}

	for _, e := range f.Entries {
		if e.Harness != "" {
			c.harnesses[e.Harness] = struct{}{}
		}
		tier, ok := parseTier(e.Tier)
		if !ok || e.ID == "" || e.Harness == "" {
			continue
		}
		m := core.Model{
			ID:      e.ID,
			Tier:    tier,
			InputM:  e.InputCostM,
			OutputM: e.OutputCostM,
			Harness: e.Harness,
		}

		if c.index[e.Harness] == nil {
			c.index[e.Harness] = make(map[core.Tier]core.Model)
		}
		// Explicit-only models are valid current selections, but never automatic targets.
		if e.Routing != "explicit_only" {
			if _, exists := c.index[e.Harness][tier]; !exists {
				c.index[e.Harness][tier] = m
			}
		}

		if c.byID[e.Harness] == nil {
			c.byID[e.Harness] = make(map[string]core.Model)
		}
		c.byID[e.Harness][e.ID] = m
		for _, alias := range e.Aliases {
			c.byID[e.Harness][alias] = m
		}

		// Build the family index for version-agnostic fallback.
		// Each family prefix is added once per (harness, family) pair.
		if e.Family != "" {
			fe := familyEntry{prefix: strings.ToLower(e.Family), model: m}
			c.byFamily[e.Harness] = append(c.byFamily[e.Harness], fe)
		}
	}
	return c
}

func mergeEntries(base, override []Entry) []Entry {
	byKey := make(map[string]Entry)
	for _, e := range override {
		if e.ID != "" && e.Harness != "" {
			byKey[entryKey(e)] = e
		}
	}
	merged := make([]Entry, 0, len(base)+len(override))
	seen := make(map[string]bool)
	for _, e := range base {
		key := entryKey(e)
		if replacement, ok := byKey[key]; ok {
			merged = append(merged, replacement)
			seen[key] = true
			continue
		}
		merged = append(merged, e)
	}
	var appended []Entry
	for key, e := range byKey {
		if !seen[key] {
			appended = append(appended, e)
		}
	}
	sort.Slice(appended, func(i, j int) bool { return entryKey(appended[i]) < entryKey(appended[j]) })
	return append(merged, appended...)
}

func validateEntries(entries []Entry) error {
	used := make(map[string]string)
	families := make(map[string][]string)
	for _, e := range entries {
		if e.ID == "" || e.Harness == "" {
			continue // section/comment entries
		}
		for _, name := range append([]string{e.ID}, e.Aliases...) {
			key := strings.ToLower(e.Harness + "\x00" + name)
			if prior, exists := used[key]; exists {
				return fmt.Errorf("duplicate catalog model or alias %q for harness %q (also %q)", name, e.Harness, prior)
			}
			used[key] = e.ID
		}
		if e.Family != "" {
			key := strings.ToLower(e.Harness)
			family := strings.ToLower(e.Family)
			for _, prior := range families[key] {
				if strings.HasPrefix(family, prior) || strings.HasPrefix(prior, family) {
					return fmt.Errorf("overlapping model families %q and %q for harness %q", family, prior, e.Harness)
				}
			}
			families[key] = append(families[key], family)
		}
	}
	return nil
}

func entryKey(e Entry) string { return e.Harness + "\x00" + e.ID }

// parseTier converts the JSON string tier to a core.Tier constant.
func parseTier(s string) (core.Tier, bool) {
	switch strings.ToLower(s) {
	case "small":
		return core.TierSmall, true
	case "mid":
		return core.TierMid, true
	case "frontier":
		return core.TierFrontier, true
	default:
		return 0, false
	}
}

// --- core.Resolver implementation ---

// ModelFor returns the model for the given harness and tier.
//
// A declared harness that lacks a model for the requested tier returns a zero
// model for that same harness. This prevents an incomplete per-harness catalog
// from sending a model ID that belongs to another tool. Only an entirely
// unknown harness falls back to the claude-code catalog.
func (c *Catalog) ModelFor(harness string, tier core.Tier) core.Model {
	if h, ok := c.index[harness]; ok {
		if m, ok := h[tier]; ok {
			return m
		}
	}
	if _, known := c.harnesses[harness]; known {
		return core.Model{Tier: tier, Harness: harness}
	}
	// Fallback: claude-code is always present in the embedded catalog.
	if h, ok := c.index["claude-code"]; ok {
		if m, ok := h[tier]; ok {
			return m
		}
	}
	// Last resort: zero model (should never happen with embedded catalog).
	return core.Model{Tier: tier, Harness: harness}
}

// LookupByID resolves a model ID to a catalog entry using three strategies,
// in priority order:
//
//  1. Exact match — the full ID matches (e.g. "claude-opus-4-8").
//  2. Alias match — the ID matches a declared alias (e.g. "claude-opus").
//  3. Family prefix match — the ID starts with the entry's family string
//     (e.g. "claude-opus-4-9" matches family "claude-opus"). Case-insensitive.
//
// OpenRouter model IDs arrive in "provider/model-id" format
// (e.g. "anthropic/claude-opus-4-8", "openai/gpt-5.3-codex"). LookupByID
// strips the provider prefix before matching, so OpenRouter works
// transparently without any special configuration from the user.
//
// This makes routing version-agnostic: a new model version is automatically
// mapped to the same tier as the previous version in the same family, without
// any catalog update.
func (c *Catalog) LookupByID(harness, modelID string) (core.Model, bool) {
	if modelID == "" {
		return core.Model{}, false
	}

	// Normalise: strip the "provider/" prefix used by OpenRouter and similar
	// meta-providers (e.g. "anthropic/claude-opus-4-8" → "claude-opus-4-8").
	normalised := normaliseModelID(modelID)

	// 1 & 2: exact / alias (already merged in byID map, O(1)).
	// Try both the original ID and the normalised form.
	for _, id := range uniqueStrings(modelID, normalised) {
		if h, ok := c.byID[harness]; ok {
			if m, ok := h[id]; ok {
				return m, true
			}
		}
	}

	// 3: family prefix — version-agnostic fallback.
	lower := strings.ToLower(normalised)
	for _, fe := range c.byFamily[harness] {
		if strings.HasPrefix(lower, fe.prefix) {
			return fe.model, true
		}
	}

	return core.Model{}, false
}

// normaliseModelID strips the "provider/" prefix from OpenRouter-style model
// IDs. "anthropic/claude-opus-4-8" → "claude-opus-4-8". IDs without a slash
// are returned unchanged.
func normaliseModelID(id string) string {
	if i := strings.Index(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// uniqueStrings returns a deduplicated slice preserving order.
func uniqueStrings(a, b string) []string {
	if a == b {
		return []string{a}
	}
	return []string{a, b}
}

// SavingsRatio returns how much cheaper `to` is versus `from` as a fraction.
func (c *Catalog) SavingsRatio(from, to core.Model) float64 {
	fromCost := from.InputM + from.OutputM
	toCost := to.InputM + to.OutputM
	if fromCost <= 0 || toCost >= fromCost {
		return 0
	}
	return (fromCost - toCost) / fromCost
}

// --- Accessors for commands (models list / check / pull) ---

// Entries returns all raw catalog entries (including unknown-tier ones).
func (c *Catalog) Entries() []Entry {
	return c.entries
}

// EffortValue translates a core.Effort to the harness-native string for a
// given model entry. Falls back to effort.String() if no map entry exists.
func EffortValue(e Entry, effort core.Effort) string {
	if e.EffortMap != nil {
		if v, ok := e.EffortMap[effort.String()]; ok {
			return v
		}
	}
	return effort.String()
}

// EntryFor returns the raw Entry for a given harness+modelID. Uses the same
// three-strategy lookup as LookupByID (exact → alias → family prefix), including
// OpenRouter "provider/model-id" normalisation.
func (c *Catalog) EntryFor(harness, modelID string) (Entry, bool) {
	if modelID == "" {
		return Entry{}, false
	}
	normalised := normaliseModelID(modelID)

	// Exact and alias match — try both original and normalised.
	for _, id := range uniqueStrings(modelID, normalised) {
		for _, e := range c.entries {
			if e.Harness != harness {
				continue
			}
			if e.ID == id {
				return e, true
			}
			for _, a := range e.Aliases {
				if a == id {
					return e, true
				}
			}
		}
	}

	// Family prefix fallback.
	lower := strings.ToLower(normalised)
	for _, e := range c.entries {
		if e.Harness == harness && e.Family != "" {
			if strings.HasPrefix(lower, strings.ToLower(e.Family)) {
				return e, true
			}
		}
	}
	return Entry{}, false
}

// IsExplicitOnly implements core.Resolver. Returns true when the model ID is
// marked routing:"explicit_only" — meaning it is a valid current selection
// but must never be chosen as an automatic routing target.
func (c *Catalog) IsExplicitOnly(harness, modelID string) bool {
	if modelID == "" {
		return false
	}
	entry, ok := c.EntryFor(harness, modelID)
	if !ok {
		return false
	}
	return entry.Routing == "explicit_only"
}
// harness-native string for the given model ID using the entry's effort_map.
// Falls back to effort.String() ("low"/"medium"/"high") when no entry or map
// is found — safe to call for any harness/model combination.
func (c *Catalog) EffortFor(harness, modelID string, effort core.Effort) string {
	if entry, ok := c.EntryFor(harness, modelID); ok {
		return EffortValue(entry, effort)
	}
	return effort.String()
}
