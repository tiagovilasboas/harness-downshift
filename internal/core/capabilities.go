// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

package core

// HarnessCapabilities describes what a harness's hook protocol can carry.
// Routing policy is shared; adapters only declare what their wire format
// supports. Add a new harness by adding a named var below — no adapter code
// needs to change.
type HarnessCapabilities struct {
	CanRewriteModel bool // hook output can replace the subagent model ID
	CanApplyEffort  bool // hook output can set reasoning effort level
}

// Per-harness capability table. Adapters import these instead of
// declaring HarnessCapabilities inline.
var (
	// ClaudeCodeCaps — PreToolUse hook honors updatedInput.model.
	ClaudeCodeCaps = HarnessCapabilities{CanRewriteModel: true, CanApplyEffort: false}

	// CursorCaps — preToolUse hook accepts updated_input.model.
	// Note: model rewrite is silently ignored on free/legacy plans.
	CursorCaps = HarnessCapabilities{CanRewriteModel: true, CanApplyEffort: false}

	// CodexCaps — PreToolUse hook accepts updatedInput.model + reasoning_effort.
	CodexCaps = HarnessCapabilities{CanRewriteModel: true, CanApplyEffort: true}
)

// RewritePlan is the concrete set of protocol actions an adapter should take.
type RewritePlan struct {
	Model            Model
	RewriteModel     bool // write Model.ID into the hook output
	ApplyEffort      bool // write effort value into the hook output
	PreserveExplicit bool // current model is explicit_only; skip all rewrites
}

// Plan translates a harness-agnostic Decision into protocol actions for the
// given harness. It consults the Resolver to detect explicit_only models so
// the plan can signal that no rewrite should happen.
//
// Passing a nil Resolver disables explicit-only detection (used in tests that
// don't need a catalog).
func (d Decision) Plan(c HarnessCapabilities, r ...Resolver) RewritePlan {
	var preserve bool
	if len(r) > 0 && r[0] != nil {
		preserve = d.ShouldPreserveExplicitModel(d.CurrentModel.ID, r[0])
	}
	if preserve {
		return RewritePlan{Model: d.Model, PreserveExplicit: true}
	}
	return RewritePlan{
		Model:        d.Model,
		RewriteModel: c.CanRewriteModel && d.ShouldRewriteModel(),
		ApplyEffort:  c.CanApplyEffort && d.ShouldApplyEffort(),
	}
}
