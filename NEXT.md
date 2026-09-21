# harness-downshift — state, gaps, and roadmap

This document tracks what is done, what is known to be incomplete,
and what comes next. Updated after each refactor wave.

---

## What was built

### Core routing engine

- `internal/core/classifier.go` — task text → Complexity (scored, deterministic, no LLM).
- `internal/core/signals.go` — `RawSignals` exported table; signals are data, not code.
- `internal/core/policy.go` — Tier, Effort, Verdict, Decision, `Route()`.
  - `ShouldRewriteModel()`, `ShouldApplyEffort()`, `ShouldPreserveExplicitModel()`.
- `internal/core/capabilities.go` — `HarnessCapabilities` with named vars per harness
  (`ClaudeCodeCaps`, `CursorCaps`, `CodexCaps`). `RewritePlan` + `Plan(caps, resolver)`.
- `internal/core/escalation.go` — `EscalationIntent` (Trivial/Normal/Review/Preserved).
  - `IntentFor(prompt, cls, Decision, Resolver)` — maps complexity + explicit_only to intent.
  - `ReviewIntent` separates deep-analysis tasks (code review, security audit, RFC) from
    complex construction tasks — both frontier tier, but purpose is now explicit.
- `internal/core/resolver.go` — `Resolver` interface covering all catalog operations.

### Catalog system

- `internal/catalog/catalog.go` — `Catalog` implements `core.Resolver`.
  - Three-layer `LookupByID`: exact → alias → family prefix.
  - OpenRouter normalisation: `anthropic/claude-opus-4-8` → `claude-opus-4-8`.
  - `IsExplicitOnly` — honors `routing: "explicit_only"` entries.
- `internal/catalog/policy.go` — `mergeEntries` + `validateEntries`.
  - Merge: override replaces matching base entries; net-new entries appended sorted.
  - Validation: no duplicate IDs/aliases per harness; no overlapping family prefixes.
- `internal/catalog/catalog.json` — embedded model data: IDs, families, costs,
  effort_maps, routing flags. Includes `gpt-6-astra` with `routing: "explicit_only"`.
- User override at `~/.harness-downshift/catalog.json` — loaded at startup, fail-open.

### Adapters

- **claudecode** — `PreToolUse` + `Task` + `updatedInput.model`
- **cursor** — `preToolUse` + `Task` + `updated_input.model`
- **codex** — `PreToolUse` + `spawn_agent` + `updatedInput.model` + `reasoning_effort`
- **grok** — config recipe only (`[subagents.roles]` in `config.toml`)

All three hook adapters:
- Call `Plan(caps, resolver)` — no routing logic, no capability checks inline.
- Set `PreserveExplicit` when current model is `explicit_only`.
- Unmarshal tool_input to `map[string]any`, mutate only `model`/`reasoning_effort`,
  re-marshal the full map — **all sibling fields preserved** (timeout, description,
  run_in_background, fork_turns, etc.).

### Infrastructure

- Single Go binary, zero runtime deps, `go:embed` for catalog.
- `goreleaser` releases: macOS arm64/amd64, Linux arm64/amd64, Windows amd64.
- `install.sh` one-liner installer with PATH guidance.
- BSL-1.1 licence with tiered liquidated damages and contributor assignment.
- `llms.txt` + `AGENTS.md` for AI agent awareness.
- `SPDX-License-Identifier: BUSL-1.1` on every Go source file.
- `models list/check/pull` — live catalog discovery from provider APIs.

### Test coverage

| Package | Coverage |
|---|---|
| `internal/adapters/claudecode` | ~91% |
| `internal/adapters/codex` | ~95% |
| `internal/adapters/cursor` | ~84% |
| `internal/catalog` | ~85% |
| `internal/core` | ~82% |
| `internal/hookutil` | 100% |
| `internal/models` | ~94% |
| `cmd/downshift` | ~70% |

---

## Architecture

```
core/
  classifier.go       — complexity from text (scored signals)
  signals.go          — RawSignals table (exported, testable, tunable)
  policy.go           — routing: tier, effort, verdict, preserve policy
  capabilities.go     — HarnessCapabilities per harness; RewritePlan; Plan()
  escalation.go       — EscalationIntent (Trivial/Normal/Review/Preserved)
  resolver.go         — Resolver interface

catalog/
  catalog.go          — Catalog struct, Load(), Resolver implementation
  policy.go           — merge + validation rules (catalog_policy)
  catalog.json        — embedded model data

adapters/
  claudecode/         — PreToolUse + Task + updatedInput.model
  cursor/             — preToolUse + Task + updated_input.model
  codex/              — PreToolUse + spawn_agent + model + reasoning_effort

cmd/downshift/        — hook runners (thin), try subcommand, models subcommands
```

**Key invariants:**
- `core` never imports `catalog` or any adapter. Dependency: `catalog → core`.
- Adapters only call `Plan(caps, resolver)` and encode the result. No routing logic.
- `explicit_only` models are never chosen automatically; preserved when current.
- `updatedInput` preserves all sibling fields — only `model` and `reasoning_effort` mutated.
- Every signal in `RawSignals` is structurally tested (compile, class, weight, no duplicates).

---

## Known gaps

| Gap | Impact | Status |
|---|---|---|
| Classifier has no feedback loop — `RawSignals` is static | Heuristic quality plateaus without real misclassification data | Open — requires production data |
| Cursor free / legacy plans silently discard `updated_input.model` | Routing runs but has no effect | Harness limitation — documented |
| Claude Code free has no subagents + blocks network | `go install` fails; nothing to route | Harness limitation — documented |

**Previously listed — now resolved:**

| Was | Resolution |
|---|---|
| `updatedInput` replaces entire tool_input — sibling fields dropped | Already correct since adapter refactor. Regression tests added to all three adapters. |
| `cmd/downshift` 0% coverage | Tests added in `fix(hooks)` (78abf8e). |

---

## What comes next

### New harnesses

The architecture is ready for more adapters. Adding one requires:
1. Research the harness's subagent hook protocol.
2. Implement `internal/adapters/<harness>/<harness>.go` — thin translator only.
3. Add catalog entries in `catalog.json`.
4. Wire a subcommand in `cmd/downshift/main.go`.

Candidates to evaluate:

| Harness | Subagents? | Hook type | Status |
|---|---|---|---|
| Gemini CLI | To verify | Unknown | Not researched |
| GitHub Copilot CLI | To verify | PreToolUse? | Not researched |
| OpenCode | To verify | Hook system | Not researched |
| Aider | No native subagents | — | Not applicable |

### Classifier feedback loop

Once users report misrouted prompts (via GitHub issues), misclassified cases
can be added to `classifier_edge_test.go` and the signals tuned accordingly.
`RawSignals` is already exported and structurally validated, so signal changes
are safe to make and test.

### Prompt for adding a new harness adapter

Use this in Claude Code or Codex. Works against the current codebase:

```
Add a new harness adapter for [HARNESS NAME] to harness-downshift.

Research:
1. Does [HARNESS NAME] have subagents? What tool name triggers a spawn?
2. Does its PreToolUse (or equivalent) hook support input rewriting (updatedInput)?
3. Does it support effort/reasoning parameters?

If rewriting is supported:
1. Create internal/adapters/<harness>/<harness>.go following the claudecode
   adapter as the template. The adapter must:
   - Decode the spawn event.
   - Call core.Route(prompt, harnessID, currentModelID, resolver).
   - Call decision.Plan(<HarnessCaps>, resolver) using a new named var in core/capabilities.go.
   - Encode only model + effort into the hook output; re-marshal the full tool_input map.
   - Fail-open on any error (return allow, exit 0).
2. Add catalog entries in internal/catalog/catalog.json.
3. Add the harness subcommand in cmd/downshift/main.go.
4. Write table-driven tests injecting catalog.Load() as the resolver.
5. Run go test -race ./... and confirm all green.
6. Do not commit or push. Report: files changed, tests passing, routing
   behaviour for trivial/normal/review tasks, and any protocol limitation.
```

---

## Immediate non-code actions

1. Upload `docs/img/social-preview.png` to GitHub Settings → Social preview
2. Register at INPI (Brazil) — ~BRL 200, Lei 9.609/98
3. Register at US Copyright Office — USD 65, unlocks USD 150k statutory damages
4. Publish a dev.to post on harness-native routing
