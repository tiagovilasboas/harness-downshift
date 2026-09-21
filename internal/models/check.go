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
)

// ProviderConfig describes one provider's model-list API.
// Exported so tests can inject mock providers pointing at httptest.Server URLs.
type ProviderConfig struct {
	Name    string
	APIURL  string
	EnvKey  string
	Harness string // which harness these models belong to
}

// defaultProviders is the production list of provider endpoints.
var defaultProviders = []ProviderConfig{
	{Name: "Anthropic", APIURL: "https://api.anthropic.com/v1/models", EnvKey: "ANTHROPIC_API_KEY", Harness: "claude-code"},
	{Name: "OpenAI", APIURL: "https://api.openai.com/v1/models", EnvKey: "OPENAI_API_KEY", Harness: "codex"},
	{Name: "xAI", APIURL: "https://api.x.ai/v1/models", EnvKey: "XAI_API_KEY", Harness: "grok"},
}

// Check queries each provider's model list API, diffs against the catalog,
// and reports known models and new/untiered ones.
func Check(cat CatalogReader, w, errW io.Writer) int {
	return CheckWithProviders(cat, defaultProviders, &http.Client{Timeout: 10 * time.Second}, w, errW)
}

// CheckWithProviders is the testable core of Check. Tests inject mock providers
// pointing at httptest.Server URLs and a pre-configured HTTP client.
func CheckWithProviders(cat CatalogReader, providers []ProviderConfig, client *http.Client, w, errW io.Writer) int {
	anyChecked := false
	anyNew := false

	for _, p := range providers {
		key := os.Getenv(p.EnvKey)
		if key == "" {
			fmt.Fprintf(w, "%-10s skipped — set %s to enable\n", p.Name, p.EnvKey)
			continue
		}

		ids, err := fetchModelIDs(client, p, key)
		if err != nil {
			fmt.Fprintf(errW, "%-10s error: %v (skipping)\n", p.Name, err)
			continue
		}
		anyChecked = true

		known, newModels := diffModels(cat, p.Harness, ids)
		fmt.Fprintf(w, "%-10s %d known", p.Name, len(known))
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
// Handles both OpenAI {"data":[{"id":"..."}]} and Anthropic {"models":[{"id":"..."}]}.
func fetchModelIDs(client *http.Client, p ProviderConfig, apiKey string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.APIURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, p.APIURL)
	}

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

// diffModels compares provider model IDs against the catalog.
func diffModels(cat CatalogReader, harness string, ids []string) (known, newModels []string) {
	for _, id := range ids {
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

// isInternalModel returns true for non-coding/chat model IDs.
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
