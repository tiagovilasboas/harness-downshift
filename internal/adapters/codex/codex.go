// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
// Commercial use requires a licence — see LICENSE for terms.

// Package codex adapts core routing decisions to OpenAI Codex CLI's hook
// protocol under multi_agent_v2.
//
// Codex spawns subagents through a reserved `spawn_agent` tool in the
// `collaboration` namespace. You cannot add a model to the provider-visible
// call (the schema is reserved), but a PreToolUse hook can inject trusted
// model + reasoning_effort metadata into the tool input via
// hookSpecificOutput.updatedInput before the local spawn handler creates the
// child. That is exactly the stage this adapter runs in.
//
// Field shape mirrors Claude Code: camelCase hookSpecificOutput with an
// updatedInput object and permissionDecision "allow". Two Codex-specific
// details:
//
//   - The envelope carries "continue": true (allow the spawn) or false (deny).
//   - The tool name may arrive as "Agent", "spawn_agent", or a flattened
//     "collaborationspawn_agent", so we match by suffix.
//
// Codex also exposes reasoning_effort as a first-class knob. Downshift sets it
// alongside the model so a trivial subagent runs cheap on both axes.
//
// Reference: developers.openai.com/codex/hooks and the community multi_agent_v2
// routing pattern.
package codex

import (
	"encoding/json"
	"strings"

	"github.com/tiagovilasboas/harness-downshift/internal/core"
	"github.com/tiagovilasboas/harness-downshift/internal/hookutil"
)

const harnessID = "codex"

// Event is the JSON Codex sends on stdin for a PreToolUse hook. Only the
// fields we need are decoded; unknown fields are ignored.
type Event struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	Model         string          `json:"model"` // the session's active model slug
}

// Output is the JSON we print on stdout to steer Codex's PreToolUse.
type Output struct {
	Continue           bool                `json:"continue"`
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput carries the PreToolUse decision. UpdatedInput replaces the
// spawn_agent tool input, injecting the routed model and reasoning effort.
type HookSpecificOutput struct {
	HookEventName            string          `json:"hookEventName"`
	PermissionDecision       string          `json:"permissionDecision"`                 // "allow" | "deny"
	PermissionDecisionReason string          `json:"permissionDecisionReason,omitempty"` // set on deny
	UpdatedInput             json.RawMessage `json:"updatedInput,omitempty"`             // rewritten spawn_agent input
}

// Handle processes a PreToolUse event. When the tool is a subagent spawn, it
// classifies the subagent's task and, if a different tier fits, rewrites the
// tool input's model and reasoning_effort via updatedInput. Returns the Output
// to print and a human-readable note (empty when nothing changed).
func Handle(ev Event) (Output, string) {
	if !isSpawnTool(ev.ToolName) {
		return allow(), ""
	}

	var ti map[string]any
	if err := json.Unmarshal(ev.ToolInput, &ti); err != nil {
		return allow(), ""
	}

	// The subagent's own task text drives its complexity. Codex v2 carries the
	// work in "message"; "task_name" is a short role/slug hint we append so a
	// name like "review_agent_payment_flow" still contributes signal.
	subPrompt := hookutil.StringField(ti, "message")
	if tn := hookutil.StringField(ti, "task_name"); tn != "" {
		subPrompt = strings.TrimSpace(subPrompt + " " + tn)
	}
	if subPrompt == "" {
		return allow(), ""
	}

	// Current model: the one already on the tool input, else the session model.
	currentModel := hookutil.StringField(ti, "model")
	if currentModel == "" {
		currentModel = ev.Model
	}

	decision := core.Route(subPrompt, harnessID, currentModel)

	// Only rewrite when a change actually helps.
	if decision.Verdict != core.VerdictDownshift && decision.Verdict != core.VerdictUpshift {
		return allow(), ""
	}

	// Inject model + reasoning effort, preserving the reserved schema fields.
	ti["model"] = decision.Model.ID
	ti["reasoning_effort"] = reasoningFor(decision.Tier)
	updated, err := json.Marshal(ti)
	if err != nil {
		return allow(), ""
	}

	out := Output{
		Continue: true,
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			UpdatedInput:       updated,
		},
	}
	return out, decision.Summary()
}

// reasoningFor maps a target tier to a Codex reasoning_effort level. Cheaper
// tiers get lower effort so a trivial subagent is cheap on both model and
// reasoning axes; the frontier tier gets high effort for genuinely hard work.
// Codex accepts minimal, low, medium, high, and xhigh.
func reasoningFor(tier core.Tier) string {
	switch tier {
	case core.TierSmall:
		return "low"
	case core.TierMid:
		return "medium"
	case core.TierFrontier:
		return "high"
	default:
		return "medium"
	}
}

// isSpawnTool reports whether the tool name is a subagent-spawning tool.
// Codex multi_agent_v2 uses "spawn_agent" (namespaced "collaboration"), which
// some builds flatten to "collaborationspawn_agent"; "Agent" is also accepted.
// We match by suffix to tolerate the namespace flattening.
func isSpawnTool(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return false
	}
	return n == "agent" || n == "spawn_agent" || strings.HasSuffix(n, "spawn_agent")
}

// allow returns a no-op PreToolUse output that lets the spawn run unchanged.
func allow() Output {
	return Output{
		Continue: true,
		HookSpecificOutput: &HookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
		},
	}
}
