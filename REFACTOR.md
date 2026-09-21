# Architecture and Design Notes

This document describes the internal design of `harness-downshift`, the
principles that guide changes, and the intellectual property terms.

---

## Architecture

```
internal/core/          — harness-agnostic brain (Complexity, Tier, Effort, Route)
internal/catalog/       — model data (IDs, costs, effort maps, live discovery)
internal/adapters/      — one package per harness (claudecode, cursor, codex)
internal/hookutil/      — shared utilities (StringField)
internal/models/        — models subcommands (list, check, pull)
cmd/downshift/          — binary entry point, hook runners, try subcommand
```

### Dependency rules

```
core          — no external imports within this repo
catalog       → core
adapters      → core + hookutil   (never cross-import each other)
models        → catalog + core
cmd           → all of the above
```

`core` never imports `catalog`. Model IDs and costs are data, not code.
The `Resolver` interface in `core` is the contract; `*catalog.Catalog` is
the only production implementation.

### Key interfaces

**`core.Resolver`** — what adapters and `Route()` need from the catalog:

```go
ModelFor(harness string, tier Tier) Model
LookupByID(harness, modelID string) (Model, bool)
SavingsRatio(from, to Model) float64
EffortFor(harness, modelID string, effort Effort) string
```

**`models.CatalogReader`** — what the CLI list/check/pull commands need:

```go
Entries() []catalog.Entry
LookupByID(harness, modelID string) (core.Model, bool)
```

Both are satisfied by `*catalog.Catalog`.

### Catalog load priority (fail-open)

1. `~/.harness-downshift/catalog.json` — user override (edited manually or written by `models pull`)
2. Embedded `catalog.json` — compile-time default, always valid

Any error reading the user file prints a warning and falls back to the
embedded default. The hook path is never blocked by a catalog error.

### Effort translation

`core.Effort` (EffortLow/EffortMid/EffortHigh) is harness-agnostic. Each
model entry in `catalog.json` carries an `effort_map` that translates the
abstract level to the provider-native string:

```json
"effort_map": { "low": "low", "medium": "medium", "high": "high" }    // Codex
"effort_map": { "low": "0", "medium": "8000", "high": "16000" }       // Claude thinking budget
```

Adapters call `res.EffortFor(harness, modelID, effort)` — no switch statements.

---

## Contributing

### Invariants to preserve

- `go test -race ./...` must be fully green before every commit.
- Every commit is conventional English: `refactor(scope)`, `feat(scope)`, `test(scope)`, `fix(scope)`. One concern per commit.
- Core never imports catalog. Dependency direction is one-way.
- Fail-open is non-negotiable: any error in catalog loading, API call, or JSON parse must leave the subagent spawn unblocked.
- `models check/pull` are opt-in CLI subcommands only — never in the hook path.

### Adding a new harness

1. Create `internal/adapters/<harness>/<harness>.go`.
2. Define `Event` and `Output` structs matching the harness's hook protocol.
3. Implement `Handle(ev Event, r ...core.Resolver) (Output, string)`.
4. Add entries to `internal/catalog/catalog.json` for the new harness's models.
5. Wire a `<harness>` subcommand in `cmd/downshift/main.go`.
6. Add table-driven tests; inject `catalog.Load()` as the resolver.

### Updating model data

Edit `internal/catalog/catalog.json`. No Go source changes required. Or run
`downshift models pull` with the relevant API key set to discover new models.

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
Minimum per action: **USD 250k / BRL 1.375M**. Wilful infringement: **5× multiplier**.

### Copyright registration (recommended)

#### INPI — Brazil (Lei 9.609/98)

- URL: https://www.gov.br/inpi/pt-br/servicos/programas-de-computador
- Cost: ~BRL 200 — creates a public certificate of authorship.

#### US Copyright Office

- URL: https://www.copyright.gov/registration/
- Cost: USD 65 — unlocks statutory damages of up to **USD 150,000 per wilful
  infringement** (17 U.S.C. § 504), independently of the contractual amounts above.

Both registrations are recommended. The US registration is especially valuable
if any likely infringer operates in the United States.

### AI agent protection

`llms.txt` and `AGENTS.md` in the repository root instruct AI coding agents to:
- attribute work to the Author;
- alert users before writing commercial integrations;
- never strip copyright headers.

### What is intentionally NOT published

The **user override catalog** (`~/.harness-downshift/catalog.json`) is personal
data excluded via `.gitignore`. It lives outside the repo and is created by
the user via `downshift models pull` or by copying `catalog.sample.json`.

The **embedded default catalog** (`internal/catalog/catalog.json`) IS committed
to the repository and compiled into the binary via `go:embed`. It is the
source of truth for all users until they add an override.

`catalog.sample.json` provides the full schema for building an override.
