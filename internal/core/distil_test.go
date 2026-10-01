// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// serveDistil starts a fake sidecar returning a fixed label and confidence.
func serveDistil(t *testing.T, label string, confidence float64, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/classify" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"label": label, "confidence": confidence})
	}))
}

func TestDistilClassify_DisabledWithoutEnv(t *testing.T) {
	t.Setenv(distilEnv, "")
	if _, _, ok := distilClassify("refactor the auth module"); ok {
		t.Error("distilClassify without DOWNSHIFT_DISTIL_URL should report ok=false")
	}
}

func TestDistilClassify_AcceptsConfidentLabel(t *testing.T) {
	srv := serveDistil(t, "COMPLEX", 0.91, http.StatusOK)
	defer srv.Close()
	t.Setenv(distilEnv, srv.URL)

	c, conf, ok := distilClassify("something the regex cannot place")
	if !ok {
		t.Fatal("expected ok=true for a confident sidecar answer")
	}
	if c != Complex {
		t.Errorf("got %s, want COMPLEX", c)
	}
	if conf != 0.91 {
		t.Errorf("got confidence %v, want 0.91", conf)
	}
}

func TestDistilClassify_RejectsLowConfidence(t *testing.T) {
	srv := serveDistil(t, "SIMPLE", 0.4, http.StatusOK)
	defer srv.Close()
	t.Setenv(distilEnv, srv.URL)

	if _, _, ok := distilClassify("vague prompt"); ok {
		t.Error("low-confidence sidecar answer should report ok=false")
	}
}

func TestDistilClassify_RejectsUnknownLabel(t *testing.T) {
	srv := serveDistil(t, "BANANA", 0.99, http.StatusOK)
	defer srv.Close()
	t.Setenv(distilEnv, srv.URL)

	if _, _, ok := distilClassify("vague prompt"); ok {
		t.Error("unknown sidecar label should report ok=false")
	}
}

func TestDistilClassify_FailsOpenOnServerError(t *testing.T) {
	srv := serveDistil(t, "COMPLEX", 0.99, http.StatusInternalServerError)
	defer srv.Close()
	t.Setenv(distilEnv, srv.URL)

	if _, _, ok := distilClassify("vague prompt"); ok {
		t.Error("5xx sidecar should report ok=false (fail-open)")
	}
}

func TestDistilClassify_FailsOpenOnUnreachable(t *testing.T) {
	t.Setenv(distilEnv, "http://127.0.0.1:1") // nothing listens here
	if _, _, ok := distilClassify("vague prompt"); ok {
		t.Error("unreachable sidecar should report ok=false (fail-open)")
	}
}

func TestClassifyTask_DistilBreaksRegexTie(t *testing.T) {
	// "help me with this" has no regex signal: v1 returns Medium, unconfident.
	srv := serveDistil(t, "TRIVIAL", 0.88, http.StatusOK)
	defer srv.Close()
	t.Setenv(distilEnv, srv.URL)

	_, c, _, confident := classifyTask("help me with this")
	if !confident {
		t.Error("sidecar-backed classification should be confident")
	}
	if c != Trivial {
		t.Errorf("got %s, want TRIVIAL from the sidecar tiebreak", c)
	}
}

func TestClassifyTask_DistilFailureKeepsV1(t *testing.T) {
	t.Setenv(distilEnv, "http://127.0.0.1:1") // unreachable: fail-open

	_, c, _, _ := classifyTask("help me with this")
	if c != Medium {
		t.Errorf("sidecar outage should keep the v1 result (MEDIUM), got %s", c)
	}
}

func TestClassifyTask_ConfidentRegexSkipsSidecar(t *testing.T) {
	// Even with a sidecar that disagrees loudly, a confident regex wins
	// without being overridden: the middle layer only breaks ties.
	srv := serveDistil(t, "TRIVIAL", 0.99, http.StatusOK)
	defer srv.Close()
	t.Setenv(distilEnv, srv.URL)

	_, c, _, _ := classifyTask("rearchitect the payment flow across services")
	if c != Complex {
		t.Errorf("confident regex (COMPLEX) should stand, got %s", c)
	}
}
