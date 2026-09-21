# Refactor Roadmap

This document captures every known clean-code debt and planned feature for
`harness-downshift`. Each wave is independently shippable and leaves the build
green and all tests passing before moving on.

Waves are ordered by impact and dependency. Waves 1–3 are pure internal cleanup
with zero functional change. Waves 4–5 add the catalog and effort systems that
make the project version-proof.

---

## Current state audit (Sep 2026)

### What works well
- `classifier.go` — single responsibility, deterministic, no external deps.
- `Decision` struct — clean value object, good Verdict enum.
- Each adapter — cohedit translation layer, explicit fail-open.
- Hook protocol coverage — Claude Code, Cursor, Codex (hook), Grok (config recipe).
- Test table structure — classifier tests are clean and data-driven.

### Known debt

| File | Principle | Problem |
|---|---|---|
| `claudecode.go`, `cursor.go`, `codex.go` | **DRY** | `stringField()` copied verbatim in all three adapters |
| `cmd/downshift/main.go` | **DRY / SRP** | `runClaudeCode`, `runCursor`, `runCodex` are 95% identical; `grokEffortFor` duplicates `reasoningFor` in codex |
| `codex.go` | **DRY** | `reasoningFor(tier)` is a local switch — will be re-implemented by every adapter that gains effort support |
| `classifier.go` | **SRP** | `Complexity.Tier()` maps complexity to routing policy — belongs in `policy.go`, not the classifier |
| `policy.go` | **SRP** | `Route()` classifies + resolves model + compares tiers — three responsibilities in one function |
| `policy.go` | **coupling** | `lookupCurrent()` iterates the in-package `Catalog` directly — core knows too much about data it should not own |
| `core/models.go` | **coupling** | `Catalog` var hardcodes model IDs as Go strings — breaks on every LLM version bump |
| `policy_test.go` | **fragility** | Asserts `"claude-haiku-4"`, `"gpt-5.6-luna"` etc. — tests break when IDs change, not when logic breaks |

---

## Wave 1 — DRY cleanup (pure refactor, no functional change)

**Goal:** remove code duplication without touching any behaviour.

### 1.1 Extract `stringField` helper

- Create `internal/hookutil/hookutil.go` with the shared `stringField(map, key)` function.
- Remove the three identical copies from `claudecode.go`, `cursor.go`, `codex.go`.
- Update imports. Run `go test -race ./...`. All green.
- **Commit:** `refactor(adapters): extract shared stringField helper into hookutil`

### 1.2 Collapse `runClaudeCode / runCursor / runCodex` in `main.go`

- Extract a generic `runHookAdapter` that accepts an unmarshal func, a handle func,
  and a fail-open printer. The three run functions collapse to three one-liners.
- **Commit:** `refactor(cmd): collapse duplicate hook-runner functions`

### 1.3 Remove `reasoningFor` from `codex.go`

After Wave 4 promotes `Effort` to core, `reasoningFor` and `grokEffortFor` in
`main.go` both disappear — they become `core.Effort.String()` translated by each
adapter. Until Wave 4 lands this stays as-is (removing it now would break codex).

---

## Wave 2 — SRP cleanup (pure refactor, no functional change)

**Goal:** each function/file has one reason to change.

### 2.1 Move `Complexity.Tier()` to `policy.go`

- The method maps a classification result to a routing tier — that is policy, not
  classification. Cut from `classifier.go`, paste above `Route()` in `policy.go`.
- Classifier no longer imports or references `Tier`.
- **Commit:** `refactor(core): move Complexity.Tier() to policy — classification should not know about tiers`

### 2.2 Decompose `Route()` into focused helpers

Current `Route()` does: classify, resolve model, compare tiers. Split into:

```go
func classifyTask(prompt string) (Complexity, Tier)
func resolveModel(harness string, tier Tier) Model          // will accept Resolver later
func compareToCurrentModel(harness, currentID string, recommended Model) (Verdict, float64)
```

Public `Route()` stays — it composes the three helpers. Tests unchanged.

- **Commit:** `refactor(core): decompose Route() into focused helpers`

---

## Wave 3 — Test hygiene (no functional change)

**Goal:** tests break only when logic breaks, not when an ID string changes.

### 3.1 Decouple `policy_test.go` from model ID strings

- `TestRoute_Downshift`: assert `Verdict == VerdictDownshift` and `Tier == TierSmall`.
  Remove `d.Model.ID != "claude-haiku-4"`.
- `TestRoute_Upshift`: assert `Verdict == VerdictUpshift` and `Tier == TierFrontier`.
- `TestRoute_Codex`: assert `Verdict == VerdictDownshift` and `Tier == TierSmall`.
- `TestSavingsRatio` / `TestModelFor_UnknownHarness`: these are catalog-data tests —
  they will move to `internal/catalog` in Wave 4.
- `TestDecisionSummary`: assert summary is non-empty, not that it contains a specific ID.
- **Commit:** `test(core): decouple policy tests from hardcoded model IDs`

---

## Wave 4 — Catalog decoupling + Effort as first-class (functional addition)

**Goal:** model IDs live in data, not in compiled Go. The build survives a model rename.

### 4.1 Promote `Effort` type to `core/models.go`

Add to `core`:

```go
type Effort int
const EffortLow, EffortMid, EffortHigh Effort = 0, 1, 2
func EffortFor(t Tier) Effort  // default mapping: small→low, mid→mid, frontier→high
```

- `Decision` struct gains `Effort Effort` field.
- `Route()` sets `decision.Effort = EffortFor(tier)`.
- `core` still does not know model IDs.
- **Commit:** `feat(core): add Effort type — harness-agnostic reasoning intensity`

### 4.2 Create `internal/catalog` package

New package owns all model-ID data. Core never imports it.

**File layout:**

```
internal/catalog/
  catalog.go          # Resolver, Load(), ModelFor(), SavingsRatio()
  catalog.json        # embedded default data
```

**`catalog.json` structure:**

```json
{
  "version": "1",
  "updated": "2026-09-20",
  "entries": [
    {
      "id": "claude-haiku-4",
      "aliases": ["claude-haiku"],
      "provider": "anthropic",
      "harness": "claude-code",
      "tier": "small",
      "effort_scale": "thinking_budget",
      "input_cost_per_1m": 0.80,
      "output_cost_per_1m": 4.00
    }
  ]
}
```

**Load priority (fail-open):**

1. `~/.harness-downshift/catalog.json` if exists and valid
2. Embedded `catalog.json` (always present, always valid)
3. On any error from file 1: log warning to stderr, silently fall back to 2

**`Resolver` interface (lives in `core`, implemented in `catalog`):**

```go
// core/resolver.go
type Resolver interface {
    ModelFor(harness string, tier Tier) (ModelRef, bool)
    SavingsRatio(from, to ModelRef) float64
}

type ModelRef struct {
    ID      string
    Tier    Tier
    Harness string
    InputM  float64
    OutputM float64
}
```

- `policy.Route()` accepts a `Resolver` parameter.
- All adapters receive a `Resolver` (injected at startup, not at import time).
- **Commit:** `feat(catalog): move model data out of core into catalog package`

### 4.3 Update adapters to use `Resolver` and `core.Effort`

- Each adapter's `Handle(ev, resolver)` replaces `core.Route(prompt, harness, current)`.
- `claudecode` + `cursor`: read `decision.Model.ID` from resolver result.
- `codex`: read `decision.Model.ID` **and** translate `decision.Effort` to
  Codex's native scale (`low/medium/high/xhigh`) — the local `reasoningFor()` switch is deleted.
- `catalog.json` gains an `effort_map` per harness (maps `low/mid/high` → native string).
- **Commit:** `feat(adapters): wire Resolver and core.Effort — remove hardcoded model IDs and effort switch`

---

## Wave 5 — Live catalog (`models` subcommand)

**Goal:** discover new models from provider APIs without re-releasing the binary.

### 5.1 `downshift models list`

Shows the effective catalog (embedded or override) as a table:

```
HARNESS       TIER      MODEL ID           IN $/1M  OUT $/1M  EFFORT
claude-code   small     claude-haiku-4     0.80     4.00      low
claude-code   mid       claude-sonnet-4    3.00     15.00     medium
...
Catalog: embedded  (override: ~/.harness-downshift/catalog.json — not found)
```

- **Commit:** `feat(cmd): add 'models list' subcommand`

### 5.2 `downshift models check`

Queries provider APIs and diffs against the effective catalog:

- Sources: OpenAI `/v1/models`, Anthropic `/v1/models`, xAI `/v1/models`
- Auth: reads `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `XAI_API_KEY` from env.
  Missing key = skip that provider silently.
- Timeout: 10 s per provider.
- On any network failure: report the error, fall back to embedded catalog for diff.
- Output: known models (in catalog), new models (not in catalog, needs human tiering).

```
Checking OpenAI... 3 known, 1 new
  + gpt-5.7-pro  → tier: unknown (needs manual assignment in catalog.json)

Checking Anthropic... 3 known, 0 new
Checking xAI... 1 known, 0 new

Run 'downshift models pull' to write the new models to your override catalog.
```

- **Commit:** `feat(cmd): add 'models check' — diff effective catalog against provider APIs`

### 5.3 `downshift models pull`

Writes/updates `~/.harness-downshift/catalog.json`:

- Merges discovered models with the current override (or creates it).
- New models added with `"tier": "unknown"` — human must assign tier.
- Existing models keep their `tier` — never overwritten.
- Idempotent: running twice produces the same result.
- **Commit:** `feat(cmd): add 'models pull' — update local catalog from provider APIs`

---

## Constraints (apply to every wave)

- `go test -race ./...` must be fully green before every commit.
- Every commit is conventional English: `refactor(scope)`, `feat(scope)`, `test(scope)`, `fix(scope)`.
- One concern per commit. No "also fixed X" bundling.
- Core never imports `catalog`. Dependency direction: `catalog` → `core`, never reversed.
- Fail-open is non-negotiable: any error in catalog loading / API call / JSON parse
  must leave the subagent spawn unblocked.
- `downshift models check/pull` are opt-in CLI subcommands only — never in the hook path.

---

## Intellectual property, licence, and registration

### Licence: Business Source License 1.1

`SPDX-License-Identifier: BUSL-1.1`

Non-commercial use (study, research, personal, open-source without revenue,
internal tooling not tied to revenue) is **free**. Commercial use requires a
written licence from Tiago de Carvalho Vilas Boas.

Change Date: **2030-09-20** → Apache 2.0.

### Damage schedule (unauthorised commercial use)

| Organisation tier | Per instance / per month |
|---|---|
| Individual / Micro | USD 10k / BRL 55k |
| Startup (5–50 / USD 250k–5M) | USD 50k / BRL 275k |
| Growth (51–500 / USD 5M–100M) | USD 150k / BRL 825k |
| Enterprise (> 500 / > USD 100M) | USD 500k / BRL 2.75M |

**Or 20% of gross monthly revenue** — whichever is greater.
Minimum per action: **USD 250k / BRL 1.375M**. Wilful: **5× multiplier**.

### Copyright registration (recommended steps)

These steps create independent legal evidence of authorship and unlock
statutory damages beyond the contractual amounts above.

#### 1. INPI — Brazil (Lei 9.609/98)

- URL: https://www.gov.br/inpi/pt-br/servicos/programas-de-computador
- Cost: ~BRL 200 (basic registration)
- Timeline: 2–3 months
- Effect: Creates a public certificate of authorship. Required for Brazilian
  court proceedings; establishes presumption of ownership.
- What to submit: source tarball of the repository at the time of filing.

#### 2. US Copyright Office

- URL: https://www.copyright.gov/registration/
- Cost: USD 65 (online, single work)
- Timeline: 3–6 months
- Effect: Unlocks statutory damages of **USD 750–USD 150,000 per infringed
  work** for wilful infringement (17 U.S.C. § 504), independently of the
  contractual amounts in LICENSE. Without registration, US courts can only
  award actual damages (much harder to prove and usually lower).
- What to submit: source code as a ZIP/tarball; register as "Literary Work —
  Computer Program".

**Recommended:** register both as soon as possible. The US registration is
especially valuable if any likely infringer operates in the United States.

### AI agent protection

`llms.txt` and `AGENTS.md` in the repository root instruct AI agents to:
- attribute work to the Author;
- alert users before writing commercial integrations;
- never strip copyright headers;
- not misrepresent the licence as MIT or Apache before 2030-09-20.

### What is intentionally NOT published

`catalog.json` (curated model tiers, effort maps, pricing) is a strategic
asset excluded from the repository via `.gitignore`. `catalog.sample.json`
provides the full schema. The real catalog lives at
`~/.harness-downshift/catalog.json` and is loaded at runtime.

---

## Progress

| Wave | Status |
|---|---|
| 1.1 Extract `stringField` | ✅ done — commit 7f35d2c |
| 1.2 Collapse hook runners | ✅ done — commit 4cd1bbc |
| 1.3 Remove `reasoningFor` (after Wave 4) | ✅ done — codex now uses `catalog.EffortValue()` |
| 2.1 Move `Complexity.Tier()` | ✅ done — commit c11f81e |
| 2.2 Decompose `Route()` | ✅ done — commit fdf1700 |
| 3.1 Decouple policy tests | ✅ done — commit bd1f372 |
| 4.1 `Resolver` interface in core | ✅ done — commit 4b35da6 |
| 4.2 Create `catalog` package | ✅ done — commit 4b35da6 |
| 4.3 `catalog.json` data | ✅ done — commit 4b35da6 |
| 4.4 Wire adapters to Resolver | ✅ done — commits aa2c40e + 9f7d961 |
| 4.5 `Effort` via catalog `effort_map` | ✅ done — commit 9f7d961 |
| 5.1 `models list` | ✅ done — commit d841cdf |
| 5.2 `models check` | ✅ done — commit 46f6878 |
| 5.3 `models pull` | ✅ done — commit 3d6e388 |
