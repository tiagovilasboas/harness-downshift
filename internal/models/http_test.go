// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package models_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/models"
)

// mockProviders returns two test providers pointing at the given servers.
// The harness names match the embedded catalog so LookupByID works.
func mockProviders(anthropicURL, openaiURL string) []models.ProviderConfig {
	return []models.ProviderConfig{
		{Name: "Anthropic", APIURL: anthropicURL, EnvKey: "ANTHROPIC_API_KEY", Harness: "claude-code"},
		{Name: "OpenAI", APIURL: openaiURL, EnvKey: "OPENAI_API_KEY", Harness: "codex"},
	}
}

func serveModels(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
}

// openAIResponse builds an OpenAI-style {"data":[{"id":"..."}]} response.
func openAIResponse(ids ...string) string {
	type model struct {
		ID string `json:"id"`
	}
	type resp struct {
		Data []model `json:"data"`
	}
	r := resp{}
	for _, id := range ids {
		r.Data = append(r.Data, model{ID: id})
	}
	b, _ := json.Marshal(r)
	return string(b)
}

// anthropicResponse builds an Anthropic-style {"models":[{"id":"..."}]} response.
func anthropicResponse(ids ...string) string {
	type model struct {
		ID string `json:"id"`
	}
	type resp struct {
		Models []model `json:"models"`
	}
	r := resp{}
	for _, id := range ids {
		r.Models = append(r.Models, model{ID: id})
	}
	b, _ := json.Marshal(r)
	return string(b)
}

// --- Check with real HTTP via httptest ---

func TestCheck_DetectsKnownAndNewModels(t *testing.T) {
	// Anthropic returns one known model + one new model (not in any known family).
	anthropicSrv := serveModels(t, anthropicResponse("claude-haiku-4-5", "claude-phoenix-1-new"))
	defer anthropicSrv.Close()

	// OpenAI returns only known models (no new).
	openaiSrv := serveModels(t, openAIResponse("gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.6-sol"))
	defer openaiSrv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "test-key")

	var out, errOut bytes.Buffer
	rc := models.CheckWithProviders(cat, mockProviders(anthropicSrv.URL, openaiSrv.URL),
		&http.Client{}, &out, &errOut)

	if rc != 0 {
		t.Errorf("rc = %d, want 0; stderr: %s", rc, errOut.String())
	}
	text := out.String()
	if !strings.Contains(text, "claude-phoenix-1-new") {
		t.Errorf("output missing new model; got:\n%s", text)
	}
	if !strings.Contains(text, "models pull") {
		t.Errorf("output should suggest 'models pull'; got:\n%s", text)
	}
}

func TestCheck_AllKnownNoNew(t *testing.T) {
	srv := serveModels(t, openAIResponse("gpt-5.6-luna", "gpt-5.6-terra", "gpt-5.6-sol"))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var out, errOut bytes.Buffer
	rc := models.CheckWithProviders(cat, mockProviders("http://unused", srv.URL),
		&http.Client{}, &out, &errOut)

	if rc != 0 {
		t.Errorf("rc = %d, want 0", rc)
	}
	if strings.Contains(out.String(), "new:") {
		t.Errorf("expected no new models; got:\n%s", out.String())
	}
}

func TestCheck_HTTP500FallsOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var out, errOut bytes.Buffer
	rc := models.CheckWithProviders(cat, mockProviders("http://unused", srv.URL),
		&http.Client{}, &out, &errOut)

	if rc != 0 {
		t.Errorf("HTTP 500 must fail-open (rc=0), got %d", rc)
	}
	if !strings.Contains(errOut.String(), "HTTP 500") {
		t.Errorf("error output should mention HTTP 500; got: %s", errOut.String())
	}
}

func TestCheck_MalformedJSONFallsOpen(t *testing.T) {
	srv := serveModels(t, `{not valid json`)
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var out, errOut bytes.Buffer
	rc := models.CheckWithProviders(cat, mockProviders("http://unused", srv.URL),
		&http.Client{}, &out, &errOut)

	if rc != 0 {
		t.Errorf("malformed JSON must fail-open, got rc=%d", rc)
	}
}

func TestCheck_FiltersInternalModels(t *testing.T) {
	// API returns both coding and internal models.
	srv := serveModels(t, openAIResponse(
		"gpt-5.6-luna",        // known coding model
		"text-embedding-3",    // embedding — should be filtered
		"tts-1",               // TTS — should be filtered
		"new-coding-model-99", // new coding model — should appear
	))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var out, errOut bytes.Buffer
	models.CheckWithProviders(cat, mockProviders("http://unused", srv.URL),
		&http.Client{}, &out, &errOut)

	text := out.String()
	if strings.Contains(text, "text-embedding") || strings.Contains(text, "tts-1") {
		t.Errorf("internal models must be filtered; got:\n%s", text)
	}
	if !strings.Contains(text, "new-coding-model-99") {
		t.Errorf("new coding model must appear; got:\n%s", text)
	}
}

func TestCheck_AnthropicModelsEnvelope(t *testing.T) {
	// Anthropic uses {"models":[...]} not {"data":[...]}
	srv := serveModels(t, anthropicResponse("claude-haiku-4-5", "claude-nova-brand-new"))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("OPENAI_API_KEY", "")

	var out, errOut bytes.Buffer
	models.CheckWithProviders(cat, mockProviders(srv.URL, "http://unused"),
		&http.Client{}, &out, &errOut)

	if !strings.Contains(out.String(), "claude-nova-brand-new") {
		t.Errorf("Anthropic-style envelope not parsed; got:\n%s", out.String())
	}
}

// --- Pull with real HTTP via httptest ---

func TestPull_WritesNewModelsToTempDir(t *testing.T) {
	// Override home to a temp directory so we don't write to the real home.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	srv := serveModels(t, openAIResponse("gpt-5.6-luna", "gpt-new-model-test"))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var out, errOut bytes.Buffer
	rc := models.PullWithProviders(cat, mockProviders("http://unused", srv.URL),
		&http.Client{}, &out, &errOut)

	if rc != 0 {
		t.Errorf("rc = %d; stderr: %s", rc, errOut.String())
	}

	// Verify the override file was written.
	catalogPath := tmpHome + "/.harness-downshift/catalog.json"
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("catalog not written: %v", err)
	}
	if !strings.Contains(string(data), "gpt-new-model-test") {
		t.Errorf("new model not in written catalog; got:\n%s", string(data))
	}
}

func TestPull_IsIdempotent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	srv := serveModels(t, openAIResponse("gpt-new-idempotent"))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	// Run twice with the same server.
	for i := range 2 {
		var out, errOut bytes.Buffer
		rc := models.PullWithProviders(cat, mockProviders("http://unused", srv.URL),
			&http.Client{}, &out, &errOut)
		if rc != 0 {
			t.Errorf("run %d: rc=%d; stderr: %s", i+1, rc, errOut.String())
		}
	}

	data, _ := os.ReadFile(tmpHome + "/.harness-downshift/catalog.json")
	// Count occurrences of the model ID — must appear exactly once.
	count := strings.Count(string(data), "gpt-new-idempotent")
	if count != 1 {
		t.Errorf("idempotency fail: model appears %d times, want 1", count)
	}
}

func TestPull_ExistingTierNotOverwritten(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Server returns a model that is already in the embedded catalog with a tier.
	srv := serveModels(t, openAIResponse("gpt-5.6-luna"))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var out, errOut bytes.Buffer
	models.PullWithProviders(cat, mockProviders("http://unused", srv.URL),
		&http.Client{}, &out, &errOut)

	// If nothing new was found, the "up to date" message should appear.
	if !strings.Contains(out.String(), "up to date") {
		// Otherwise the catalog was written — verify the existing tier is preserved.
		if data, err := os.ReadFile(tmpHome + "/.harness-downshift/catalog.json"); err == nil {
			if strings.Contains(string(data), `"tier": "unknown"`) &&
				strings.Contains(string(data), "gpt-5.6-luna") {
				t.Errorf("existing model tier should not be set to unknown; got:\n%s", string(data))
			}
		}
	}
}

func TestPull_HTTP500FallsOpen(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("ANTHROPIC_API_KEY", "")

	var out, errOut bytes.Buffer
	rc := models.PullWithProviders(cat, mockProviders("http://unused", srv.URL),
		&http.Client{}, &out, &errOut)

	if rc != 0 {
		t.Errorf("HTTP 500 must fail-open, got rc=%d", rc)
	}
}
