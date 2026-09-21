// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
)

// providerConfig describes one provider's model-list API.
type providerConfig struct {
	name    string
	apiURL  string
	envKey  string
	harness string // which harness these models belong to
}

var providers = []providerConfig{
	{
		name:    "Anthropic",
		apiURL:  "https://api.anthropic.com/v1/models",
		envKey:  "ANTHROPIC_API_KEY",
		harness: "claude-code",
	},
	{
		name:    "OpenAI",
		apiURL:  "https://api.openai.com/v1/models",
		envKey:  "OPENAI_API_KEY",
		harness: "codex",
	},
	{
		name:    "xAI",
		apiURL:  "https://api.x.ai/v1/models",
		envKey:  "XAI_API_KEY",
		harness: "grok",
	},
}

// Check queries each provider's model list API, diffs it against the effective
// catalog, and reports known models and new/untiered models.
//
// Missing API keys skip the provider silently (just a note).
// Network errors, timeouts, or unexpected responses are reported but never
// block — fall back continues with remaining providers.
//
// Returns exit code 0 on success (even if new models found), 1 only on a
// hard error that prevented all provider checks.
func Check(cat *catalog.Catalog, w, errW io.Writer) int {
	client := &http.Client{Timeout: 10 * time.Second}
	anyChecked := false
	anyNew := false

	for _, p := range providers {
		key := os.Getenv(p.envKey)
		if key == "" {
			fmt.Fprintf(w, "%-10s skipped — set %s to enable\n", p.name, p.envKey)
			continue
		}

		ids, err := fetchModelIDs(client, p, key)
		if err != nil {
			fmt.Fprintf(errW, "%-10s error: %v (skipping)\n", p.name, err)
			continue
		}
		anyChecked = true

		known, newModels := diffModels(cat, p.harness, ids)
		fmt.Fprintf(w, "%-10s %d known", p.name, len(known))
		if len(newModels) == 0 {
			fmt.Fprintln(w, ", 0 new ✓")
		} else {
			fmt.Fprintf(w, ", %d new:\n", len(newModels))
			for _, id := range newModels {
				fmt.Fprintf(w, "  + %-40s tier: unknown (run 'downshift models pull' to add)\n", id)
			}
			anyNew = true
		}
	}

	if !anyChecked {
		fmt.Fprintln(w, "\nNo providers checked. Set at least one API key:")
		fmt.Fprintln(w, "  ANTHROPIC_API_KEY, OPENAI_API_KEY, XAI_API_KEY")
		return 0
	}

	if anyNew {
		fmt.Fprintln(w, "\nRun 'downshift models pull' to add new models to your catalog.")
		fmt.Fprintln(w, "New models are added with tier=unknown — assign their tier before use.")
	}
	return 0
}

// fetchModelIDs calls a provider's /v1/models endpoint and returns model IDs.
// It handles both OpenAI-style {"data":[{"id":"..."}]} and
// Anthropic-style {"models":[{"id":"..."}]} responses.
func fetchModelIDs(client *http.Client, p providerConfig, apiKey string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	// Anthropic also requires this header.
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, p.apiURL)
	}

	// Parse into a generic envelope: accepts both "data" and "models" arrays.
	var envelope struct {
		Data   []struct{ ID string `json:"id"` } `json:"data"`
		Models []struct{ ID string `json:"id"` } `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	seen := make(map[string]bool)
	var ids []string
	for _, m := range envelope.Data {
		if m.ID != "" && !seen[m.ID] {
			ids = append(ids, m.ID)
			seen[m.ID] = true
		}
	}
	for _, m := range envelope.Models {
		if m.ID != "" && !seen[m.ID] {
			ids = append(ids, m.ID)
			seen[m.ID] = true
		}
	}
	return ids, nil
}

// diffModels compares a list of provider model IDs against the catalog.
// Returns (known IDs, new IDs not yet in the catalog).
func diffModels(cat *catalog.Catalog, harness string, ids []string) (known, newModels []string) {
	for _, id := range ids {
		// Skip internal/deprecated slugs (embeddings, tts, dall-e, etc.)
		if isInternalModel(id) {
			continue
		}
		if _, ok := cat.LookupByID(harness, id); ok {
			known = append(known, id)
		} else {
			newModels = append(newModels, id)
		}
	}
	return
}

// isInternalModel returns true for model IDs that are not coding/chat models
// and should not appear in the catalog (embeddings, TTS, image gen, etc.).
func isInternalModel(id string) bool {
	lower := strings.ToLower(id)
	skip := []string{"embed", "tts", "dall-e", "whisper", "davinci", "babbage",
		"curie", "ada", "moderation", "realtime", "audio", "search"}
	for _, s := range skip {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
