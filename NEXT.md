# What was built — and what comes next

This document captures what harness-downshift is today, the honest gaps,
and the architectural direction for the next phase.

---

## What was built (v0.1.0-beta.1)

### Core routing engine

- `internal/core` — harness-agnostic brain: `Complexity`, `Tier`, `Effort`,
  `Verdict`, `Decision`, `Route()`.
- Deterministic classifier: scored keyword signals, tie-breaks toward higher
  complexity, no LLM in the loop.
- `Effort` as a first-class dimension alongside `Tier` — cheap work runs low
  reasoning, hard work runs high, regardless of harness.

### Catalog system

- `internal/catalog` — model data lives in JSON, not in Go source.
- Embedded `catalog.json` compiled into the binary via `go:embed`.
- User override at `~/.harness-downshift/catalog.json` — loaded at startup,
  fail-open to embedded on any error.
- `LookupByID`: three-layer matching — exact → alias → family prefix.
  Version-agnostic: `claude-opus-4-9` matches family `claude-opus` automatically.
- OpenRouter normalisation: `anthropic/claude-opus-4-8` → strips prefix →
  matches as `claude-opus-4-8`. Transparent for users routing through
  meta-providers.
- `EffortFor(harness, modelID, effort)` translates abstract effort to the
  harness-native string via per-entry `effort_map` in the catalog.
- `models list/check/pull` — live catalog discovery from provider APIs
  (OpenAI, Anthropic, xAI) with timeout, fail-open, and idempotent pull.

### Adapters

- **claudecode** — `PreToolUse` + `Task` matcher + `updatedInput.model`
- **cursor** — `preToolUse` + `Task` matcher + `updated_input.model`
- **codex** — `PreToolUse` + `spawn_agent` matcher + `updatedInput.model`
  + `reasoning_effort` from catalog `effort_map`
- **grok** — config recipe only (`[subagents.roles]` in `config.toml`).
  Grok's hook API is allow/deny only; no `updatedInput` support.

### Infrastructure

- Single Go binary, zero runtime deps, `go:embed` for catalog.
- `goreleaser` releases: macOS arm64/amd64, Linux arm64/amd64, Windows amd64.
- `install.sh` one-liner installer with PATH guidance.
- BSL-1.1 licence with tiered liquidated damages and contributor assignment.
- `llms.txt` + `AGENTS.md` for AI agent awareness and commercial use alerts.
- `SPDX-License-Identifier: BUSL-1.1` on every Go source file.

### Test coverage

| Package | Coverage |
|---|---|
| `internal/adapters/claudecode` | ~91% |
| `internal/adapters/codex` | ~95% |
| `internal/adapters/cursor` | ~84% |
| `internal/catalog` | ~83% |
| `internal/core` | ~80% |
| `internal/hookutil` | 100% |
| `internal/models` | ~94% |

---

## Known gaps (technical)

| Gap | Impact | Effort |
|---|---|---|
| `updatedInput` replaces entire tool_input — sibling fields like `timeout` or `run_in_background` are silently dropped | Subagents using non-standard fields may behave unexpectedly | Medium |
| `cmd/downshift` has 0% test coverage | Binary entrypoint has no automated tests | Medium |
| Classifier has no feedback loop | Signals are static; can't learn from real misclassifications in production | Large |
| Cursor free / legacy plans silently discard `updated_input.model` | Routing appears to work but has no effect | Harness limitation — documented |
| Claude Code free has no subagents + blocks network | `go install` fails; no subagents to route | Harness limitation — documented |

---

## Next phase — policy separation

The current design works but mixes routing policy into multiple places:
- Catalog knows about harness capabilities implicitly (via `effort_map` presence)
- Adapters each re-implement effort translation logic
- The concept of "never automatically choose a top model unless explicitly requested" is not expressed anywhere

The next architectural split:

### `core/` sub-packages

```
core/
  classification/    — complexity from text (current classifier.go)
  routing_policy/    — recommended tier + effort from complexity
  capabilities/      — per-harness: can_rewrite_model, can_rewrite_effort, hook_mechanism
  escalation_policy/ — when to never auto-choose a top model; explicit-only models
```

**`capabilities`** would replace per-adapter `if` logic with a central table:

```go
type HarnessCapabilities struct {
    CanRewriteModel  bool
    CanRewriteEffort bool
    HookMechanism   string // "PreToolUse", "config", etc.
}
```

**`escalation_policy`** would express the rule:
> `gpt-6-astra` (or any `explicit_only` model) is never chosen automatically.
> If the current model is astra, preserve it; if no model is specified, route
> to the tier's default, not to astra.

This becomes `ShouldPreserveExplicitModel(decision, currentModelID) bool` in core.

### `catalog/catalog_policy`

Extract from `catalog.go` into a dedicated module:
- Override merge validation (no duplicate family conflicts)
- Cross-harness model leakage prevention
- Safe fallback chain
- Deterministic entry ordering

### Adapters become pure protocol translators

After the above, each adapter reduces to:
1. Decode event
2. Call `core.Route(prompt, harness, currentID, resolver)`
3. Check `capabilities[harness].CanRewriteModel`
4. If yes, encode `updatedInput` with `decision.Model.ID`
5. If effort supported, encode `decision.Effort` via `resolver.EffortFor(...)`
6. Fail-open

No routing logic, no capability checks, no policy — just encoding.

### `explicit_only` catalog field

Add to `catalog.json` entries:

```json
{
  "id": "gpt-6-astra",
  "family": "gpt-6-astra",
  "explicit_only": true,
  ...
}
```

The routing engine checks this before assigning a target:
if `entry.ExplicitOnly && currentModelID != entry.ID` → skip this model for routing.

---

## Next harnesses to evaluate

| Harness | Subagents? | Hook type | Likely mechanism |
|---|---|---|---|
| Gemini CLI | To verify | Unknown | To research |
| Aider | No native subagents | — | Not applicable |
| Continue (VS Code) | To verify | Plugin API | To research |
| GitHub Copilot CLI | To verify | PreToolUse? | To research |
| OpenCode | To verify | Hook system | To research |

---

## Prompt for adding a new harness (after policy separation is done)

Once `HarnessCapabilities` and `escalation_policy` exist, use this prompt in
Claude Code / Codex to wire a new adapter correctly:

```
Continue the harness-downshift refactor for [HARNESS NAME].

Context:
- Shared routing policy is in internal/core:
  - Decision.ShouldRewriteModel()
  - Decision.ShouldApplyEffort()
  - Decision.Plan(core.HarnessCapabilities)
- Adapters must remain thin protocol translators only.
- The shared policy applies:
  - trivial work → small/low
  - normal implementation → mid/medium
  - code review / complex analysis → frontier/high
- explicit_only models are never chosen automatically; preserve only when
  the user explicitly selected them.
- Unknown source models still receive the recommended target model.
- Keep catalog policy inside internal/catalog.
- Do not alter files outside the repository.
- Add focused regression tests and run `go test -race ./...`.
- Build the artifact and report the exact hook/config required.
- Do not commit or push. Return: files changed, tests run, routing behavior
  for trivial/normal/code-review/explicit tasks, and any protocol limitation.
```

**Note:** this prompt only works correctly after the policy separation above
is implemented. Running it against the current codebase will generate broken
code referencing non-existent methods.

---

## Immediate actions (before next code phase)

1. Upload `docs/img/social-preview.png` to GitHub Settings → Social preview
2. Register at INPI (Brazil) — ~BRL 200, Lei 9.609/98
3. Register at US Copyright Office — USD 65, unlocks USD 150k statutory damages
4. Publish a dev.to post on harness-native routing
5. Respond to Anthropic issue #69545 (done) + monitor #95769 for engagement
