// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

// Distil sidecar: the fixed middle layer of the classification cascade.
//
// Cascade order (cheap to expensive):
//
//	regex/softmax (microseconds) -> DistilBERT sidecar (tens of ms) -> Laya/Jev (ambiguous) -> LLM (never, for routing)
//
// The sidecar is a local DistilBERT fine-tuned on the four complexity
// labels (TRIVIAL/SIMPLE/MEDIUM/COMPLEX), served by scripts/distil_serve.py.
// It is consulted ONLY when the regex classifier is not confident, and it
// fails open: any error keeps the v1 regex result. No new Go dependencies —
// stdlib HTTP client only.
//
// Enable with:
//
//	DOWNSHIFT_DISTIL_URL=http://localhost:8001 downshift try "..."

// distilEnv is the base URL of the DistilBERT sidecar. Empty means disabled.
const distilEnv = "DOWNSHIFT_DISTIL_URL"

// distilClassifyPath is the sidecar endpoint, appended to the base URL.
const distilClassifyPath = "/classify"

// distilTimeout bounds the sidecar call so the hook never blocks a spawn.
// The hook contract is fail-open with a deadline; 300ms keeps DistilBERT
// (tens of ms on CPU) comfortably inside it.
const distilTimeout = 300 * time.Millisecond

// distilMinConfidence is the minimum sidecar confidence to accept its label.
// Below this the v1 regex result stands. Tune per deployment: a lower
// threshold routes more through the middle layer, a higher one keeps v1.
const distilMinConfidence = 0.6

// distilRequest is the sidecar request body.
type distilRequest struct {
	Text string `json:"text"`
}

// distilResponse is the sidecar response body.
type distilResponse struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
}

// distilClassify asks the sidecar for a complexity label. It returns the
// complexity, the raw confidence, and whether the answer is usable.
// ok=false on every failure mode (disabled, timeout, bad status, unknown
// label, low confidence) so callers can fall through to the v1 result.
func distilClassify(prompt string) (Complexity, float64, bool) {
	base := strings.TrimRight(os.Getenv(distilEnv), "/")
	if base == "" {
		return Medium, 0, false
	}

	body, err := json.Marshal(distilRequest{Text: prompt})
	if err != nil {
		return Medium, 0, false
	}

	client := &http.Client{Timeout: distilTimeout}
	resp, err := client.Post(base+distilClassifyPath, "application/json", bytes.NewReader(body))
	if err != nil {
		return Medium, 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Medium, 0, false
	}

	var out distilResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Medium, 0, false
	}

	c, known := parseDistilLabel(out.Label)
	if !known {
		return Medium, 0, false
	}
	if out.Confidence < distilMinConfidence {
		return Medium, out.Confidence, false
	}
	return c, out.Confidence, true
}

// parseDistilLabel maps a sidecar label to Complexity. Labels match
// Complexity.String(), case-insensitive, so training and serving stay 1:1.
func parseDistilLabel(label string) (Complexity, bool) {
	switch strings.ToUpper(strings.TrimSpace(label)) {
	case "TRIVIAL":
		return Trivial, true
	case "SIMPLE":
		return Simple, true
	case "MEDIUM":
		return Medium, true
	case "COMPLEX":
		return Complex, true
	default:
		return Medium, false
	}
}
