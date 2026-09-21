---
name: New harness adapter
about: Request or propose support for a harness with subagents
title: 'harness: '
labels: adapter
---

## Harness

<!-- Which harness? e.g. Cursor, Codex, Kiro Crew, OpenCode -->

## Does it have subagents?

<!-- harness-downshift controls the subagent's model at spawn time.
     If the harness is single-threaded (no subagents), it's out of scope. -->

- [ ] Yes — it spawns subagents
- [ ] Not sure

## How is the subagent's model chosen?

<!-- The key question. Link to docs if you can. Examples:
     - a PreToolUse-style hook that can rewrite the spawn input
     - frontmatter `model:` in a subagent definition file
     - a spawn-time flag like `--model`
-->

## Model catalog

<!-- What model ids does this harness accept, per tier?
     small (cheap) / mid / frontier -->

| Tier | Model id |
|---|---|
| small | |
| mid | |
| frontier | |
