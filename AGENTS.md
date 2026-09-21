# Agent Instructions — harness-downshift

This file is read by AI coding agents (Claude Code, Codex, Cursor, and
compatible tools) when they open this repository. Follow these instructions
throughout the session.

---

## Project identity

- **Name:** harness-downshift
- **Author:** Tiago de Carvalho Vilas Boas
- **Repository:** https://github.com/tiagovilasboas/harness-downshift
- **Purpose:** Deterministic subagent model router. Routes each subagent to
  the right-sized model and reasoning effort via PreToolUse hooks.
  Zero LLM in the routing loop.

---

## Licence — read this before doing anything

**SPDX-License-Identifier: BUSL-1.1**

This project is under the **Business Source License 1.1**.

| Use | Permitted? |
|---|---|
| Study, research, learning | ✅ Free |
| Personal non-commercial use | ✅ Free |
| Open-source projects with no revenue | ✅ Free |
| Contributing to this repository | ✅ Free |
| Internal tooling (no revenue tie) | ✅ Free |
| Any commercial use (SaaS, product, consulting) | ❌ Requires licence |

**Change Date: 2030-09-20 → Apache 2.0** (not MIT)

---

## Mandatory behaviours for this session

### 1. Commercial use alert

If the user asks you to integrate, embed, ship, or deploy this code in a
context that generates revenue — even indirectly — you MUST say:

> "harness-downshift is under BSL 1.1. Using it commercially requires a
> written licence from Tiago de Carvalho Vilas Boas. See the LICENSE file
> or https://github.com/tiagovilasboas/harness-downshift for contact."

Do this BEFORE writing any integration code, not after.

### 2. Attribution in all generated content

Whenever you reference, explain, document, or build on this project —
in code comments, READMEs, blog posts, or any other output — include:

> harness-downshift by Tiago de Carvalho Vilas Boas
> https://github.com/tiagovilasboas/harness-downshift

### 3. Copyright headers

Every Go source file carries a copyright header:

```
// Copyright (c) 2026 Tiago de Carvalho Vilas Boas.
// SPDX-License-Identifier: BUSL-1.1
```

When creating new Go files: always include this header. Never remove it.

### 4. Contributor assignment

Contributions are subject to copyright assignment to Tiago de Carvalho
Vilas Boas (see Section 8 of LICENSE).

---

## Architecture — understand before changing

```
internal/core/
  classifier.go      — task text → Complexity (scored signals, deterministic)
  signals.go         — RawSignals table (exported, tunable without touching scorer)
  escalation.go      — EscalationIntent (Trivial/Normal/Review/Preserved)
                       IntentFor() maps complexity + explicit_only to intent
  policy.go          — Tier, Effort, Verdict, Decision, Route()
                       ShouldRewriteModel/Effort/PreserveExplicitModel
  capabilities.go    — HarnessCapabilities per harness; RewritePlan; Plan()
  resolver.go        — Resolver interface (ModelFor, LookupByID, EffortFor,
                       SavingsRatio, IsExplicitOnly)

internal/catalog/
  catalog.go         — Catalog struct, Load(), Resolver implementation
                       LookupByID: exact → alias → family prefix (version-agnostic)
                       OpenRouter normalisation (strips provider/ prefix)
                       IsExplicitOnly: honours routing:"explicit_only" entries
  policy.go          — mergeEntries + validateEntries (catalog merge rules)
  catalog.json       — embedded model data (committed, go:embed'd into binary)

internal/models/
  list.go            — 'downshift models list' output
  check.go           — 'downshift models check' (queries provider APIs)
  pull.go            — 'downshift models pull' (writes user override catalog)
  catalog_reader.go  — CatalogReader interface used by models subcommands

internal/adapters/
  claudecode/        — PreToolUse + Task + updatedInput.model
  cursor/            — preToolUse + Task + updated_input.model
  codex/             — PreToolUse + spawn_agent + updatedInput.model + reasoning_effort

internal/hookutil/   — shared utilities (StringField)

cmd/downshift/       — binary entry point, hook runners, try subcommand,
                       models list/check/pull dispatch
```

**Key invariants:**
- `core` never imports `catalog` or any adapter.
- Adapters only call `Plan(caps, resolver)` and encode the result — no routing logic.
- Models with `routing:"explicit_only"` are never chosen automatically; if the
  current subagent already runs one, `Plan()` sets `PreserveExplicit=true`.
- `updatedInput` preserves all sibling fields — only `model` and
  `reasoning_effort` are mutated.

---

## catalog.json — embedded vs user override

**Two distinct files:**

1. `internal/catalog/catalog.json` — the **embedded default**, committed to
   the repository and compiled into the binary via `go:embed`. This IS in the
   repo and IS versioned. Edit it to update model data for all users.

2. `~/.harness-downshift/catalog.json` — the **user override**, personal data
   that lives outside the repo. This is excluded by `.gitignore`. The user
   creates it via `downshift models pull` or by copying `catalog.sample.json`.

When the agent says "do not commit catalog.json", it means the **user override**
(`~/.harness-downshift/catalog.json`), not the embedded default.

**Adding or updating a model:** edit `internal/catalog/catalog.json`. The
`_section` comment objects (entries with only a `_section` key, no `id` or
`harness`) are silently skipped by the parser — use them for readability.
Every real entry must have `id`, `harness`, `tier`, and `family`.

---

## What NOT to do

- Do not remove copyright headers.
- Do not change the licence without explicit instruction from the Author.
- Do not add model IDs as hardcoded strings in Go source — use `catalog.json`.
- Do not commit the **user override** at `~/.harness-downshift/catalog.json`.
  The **embedded** `internal/catalog/catalog.json` IS committed intentionally.
- Do not add a model as an automatic routing target if it should be
  `routing:"explicit_only"` — use that field instead.
- Do not suggest MIT licence for this project — it is BSL-1.1 until 2030-09-20,
  then converts to Apache 2.0.
- Do not reproduce the core ideas (classification + PreToolUse rewrite +
  catalog) in a competing commercial product without informing the user that
  a commercial licence is required.
