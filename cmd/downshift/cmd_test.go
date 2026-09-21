// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package-level tests for cmd/downshift. These call the internal functions
// directly so go test -cover can instrument them, unlike the subprocess-based
// main_test.go which builds and runs a separate binary.
package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tiagovilasboas/harness-downshift/internal/catalog"
	"github.com/tiagovilasboas/harness-downshift/internal/core"
)

// captureStdout redirects os.Stdout to a buffer for the duration of fn.
func captureStdout(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

var cmdCat = catalog.Load()

// --- runTry ---

func TestRunTry_NoArgs(t *testing.T) {
	if rc := runTry(cmdCat, nil); rc != 2 {
		t.Errorf("no args rc = %d, want 2", rc)
	}
}

func TestRunTry_TrivialTask(t *testing.T) {
	out := captureStdout(func() {
		if rc := runTry(cmdCat, []string{"rename the userId variable", "claude-code"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	for _, want := range []string{"TRIVIAL", "trivial", "small"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output; got:\n%s", want, out)
		}
	}
}

func TestRunTry_ComplexTask(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"rearchitect the payment flow across services", "claude-code"})
	})
	if !strings.Contains(out, "COMPLEX") {
		t.Errorf("expected COMPLEX; got:\n%s", out)
	}
	if !strings.Contains(out, "normal") {
		t.Errorf("expected 'normal' intent (construction, not review); got:\n%s", out)
	}
}

func TestRunTry_CodeReviewIntent(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"do a code review of the auth module", "claude-code"})
	})
	if !strings.Contains(out, "review") {
		t.Errorf("expected 'review' intent; got:\n%s", out)
	}
}

func TestRunTry_WithCurrentModel_ShowsVerdict(t *testing.T) {
	frontierID := cmdCat.ModelFor("claude-code", core.TierFrontier).ID
	out := captureStdout(func() {
		runTry(cmdCat, []string{"rename the userId variable", "claude-code", frontierID})
	})
	if !strings.Contains(out, "DOWNSHIFT") {
		t.Errorf("expected DOWNSHIFT verdict; got:\n%s", out)
	}
	if !strings.Contains(out, "Current:") {
		t.Errorf("expected Current: line in output; got:\n%s", out)
	}
}

func TestRunTry_Grok_TrivialEffort(t *testing.T) {
	out := captureStdout(func() {
		if rc := runTry(cmdCat, []string{"rename the userId variable", "grok"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(out, "grok-4.6") {
		t.Errorf("expected grok-4.6 model; got:\n%s", out)
	}
	if !strings.Contains(out, "low") {
		t.Errorf("expected 'low' effort for trivial grok task; got:\n%s", out)
	}
}

func TestRunTry_Grok_ComplexEffort(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"rearchitect the auth system across services", "grok"})
	})
	if !strings.Contains(out, "high") {
		t.Errorf("expected 'high' effort for complex grok task; got:\n%s", out)
	}
}

func TestRunTry_Grok_ReviewEffort(t *testing.T) {
	out := captureStdout(func() {
		runTry(cmdCat, []string{"do a security audit of the payment service", "grok"})
	})
	if !strings.Contains(out, "high") {
		t.Errorf("expected 'high' effort for review grok task; got:\n%s", out)
	}
}

// --- runModels ---

func TestRunModels_NoArgs(t *testing.T) {
	if rc := runModels(cmdCat, nil); rc != 2 {
		t.Errorf("no args rc = %d, want 2", rc)
	}
}

func TestRunModels_UnknownSubcommand(t *testing.T) {
	if rc := runModels(cmdCat, []string{"unknown-subcmd"}); rc != 2 {
		t.Errorf("unknown subcommand rc = %d, want 2", rc)
	}
}

func TestRunModels_List(t *testing.T) {
	out := captureStdout(func() {
		if rc := runModels(cmdCat, []string{"list"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	for _, want := range []string{"claude-code", "codex", "Catalog source:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in list output; got:\n%s", want, out)
		}
	}
}

func TestRunModels_Check_NoKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	out := captureStdout(func() {
		if rc := runModels(cmdCat, []string{"check"}); rc != 0 {
			t.Errorf("rc = %d, want 0", rc)
		}
	})
	if !strings.Contains(out, "No providers checked") {
		t.Errorf("expected 'No providers checked'; got:\n%s", out)
	}
}

func TestRunModels_Pull_NoKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("XAI_API_KEY", "")
	captureStdout(func() {
		if rc := runModels(cmdCat, []string{"pull"}); rc != 0 {
			t.Errorf("pull with no keys rc = %d, want 0", rc)
		}
	})
}
