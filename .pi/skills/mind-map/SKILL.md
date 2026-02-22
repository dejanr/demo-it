---
name: mind-map
description: "Create and update MIND_MAP.md - graph-based project documentation using [N] node references."
---

# Mind Map Skill

A graph-based documentation format stored as plain text where each node is a single line with inline `[N]` references for navigation.

## Purpose

`MIND_MAP.md` is an **architecture and capability index**, not an implementation changelog.
It is a high-level graph of the project architecture and setup, optimized for quick navigation to the right subsystem/context.
Use it to keep a stable mental model of the system.

## Format

```text
[N] **Node Title** - Description with [N] inline references to other nodes.
```

- Each node is one line
- `[N]` creates bidirectional navigation
- References embedded naturally in descriptive text

## File Location

Store as `MIND_MAP.md` at the project root.

## Structure

### Overview Nodes [1-9]

Navigation hubs—read these first:

- [1] Always the project entry point, links to all major areas
- [2-9] Major subsystems, architectural layers, or cross-cutting concerns

### Detail Nodes [10+]

Cluster around parent concepts:

- Link back to parent overview node
- Cross-link to related detail nodes
- Add only when a concept is durable and worth navigating to later

## Level of Detail (Important)

Keep nodes **high signal and stable**.

Include:

- Domain capabilities
- Architectural boundaries
- Key runtime decisions
- Stable integration points

Avoid:

- Exhaustive implementation details
- Temporary debugging notes
- Full lists of flags/env vars/shortcuts unless they are core behavior
- Commit-by-commit deltas
- Tautological statements that encode obvious best practices without project-specific context (e.g., “dependencies follow framework compatibility ranges”)
- Runbook/command-level details that already belong in README, `devenv.nix`, scripts, or shell help

Rule of thumb:

- If it belongs in code comments, commit history, or README usage docs, keep it out of the mind map.
- Prefer concise summaries like “global shortcut support with fallback” over enumerating every key combo.
- Mind map entries should describe what the system is/can do, not how to run it step-by-step.

## Creating

1. Read the codebase structure first
2. Identify major architectural areas (up to 9)
3. Write overview nodes [1-9] covering these areas
4. Add detail nodes [10+] clustered around each area
5. Ensure every node has at least one incoming and outgoing reference

## Updating

Update `MIND_MAP.md` **only** when there is a durable change to:

- architecture boundaries or subsystem relationships
- project setup/development workflow structure
- core runtime/integration topology

Do **not** update it for routine implementation churn:

- bug fixes
- small refactors
- UI text/layout tweaks
- one-off error fixes
- short-lived technical workarounds

Process:

1. Read current `MIND_MAP.md`
2. Follow references to find affected nodes
3. Update only the minimum set of impacted nodes
4. Add nodes only for genuinely new, durable concepts
5. Prefer replacing verbose text with compact architectural summaries
6. Keep wording future-proof and avoid transient details
7. Batch related edits together; avoid micro-updates after minor code changes

## Navigation

```bash
grep "^\[17\]" MIND_MAP.md      # Find node by ID
grep "\[17\]" MIND_MAP.md       # Find all references to node
grep -c "^\[" MIND_MAP.md       # Count nodes
```
