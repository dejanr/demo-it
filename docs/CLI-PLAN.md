# CLI/daemon implementation plan (partial)

## Goal

Build a Go boundary where `demo-itd` is the single authority for run state and execution, and `demo-it` is the only control surface consumed by Neovim.

Neovim scope is intentionally small:

- keymaps/commands
- invoking `demo-it`
- rendering slides/speaker notes

## Scope split

### Backend (`demo-itd`)

- run lifecycle: create, load, lock, persist
- transcript parse + revision hash
- interaction state machine
- focus policy + tmux handoff
- idempotent execution ledger
- speaker-note artifact generation

### CLI (`demo-it`)

- thin transport client to daemon
- deterministic command UX
- optional local fallback for daemon startup

## Initial commands

- `demo-it start`
- `demo-it status` (managed workspace/session status)
- `demo-it run-status` (daemon run state)
- `demo-it reload`
- `demo-it next`
- `demo-it prev`
- `demo-it rerun`
- `demo-it jump --slide <id|index>`
- `demo-it focus --policy <present|return|none>`
- `demo-it record [--title <text>] [--yes] [-f <file.md>] [workspace-path]` (stdout remains redirect-friendly)

## State and context boundaries

### Run context

- `run_id`
- `repo_root`
- `socket_path`
- `transcript_path`
- `default_focus_policy`

### State

- cursor: `current_slide`, `current_interaction`
- transcript model: slides + interactions
- history stack for reversible transitions
- execution ledger keyed by `idempotency_key`
- status: `idle|running|completed|failed`

## Transition rules (first cut)

- `next`: execute next interaction; if slide interactions exhausted, move to next slide and reset interaction pointer
- `prev`: restore last cursor snapshot from history
- `jump`: set cursor to selected slide, interaction to `-1`
- `rerun`: force execution for current interaction
- `reload`: reparse transcript, reconcile ledger by hash, keep cursor when possible

## Test strategy (strong borders)

1. Protocol validation tests
   - invalid envelope
   - unsupported command
   - invalid focus policy
2. State machine transition tests
   - next/prev/jump/rerun rules
   - boundary behavior at first/last slide
3. Ledger reconciliation tests
   - unchanged hash keeps completion
   - changed hash invalidates completion
4. Concurrency/lock tests (later)
   - only one execution transition at a time

## Milestones

### M1

- protocol package + tests ✅
- state machine package + tests ✅

### M2

- daemon skeleton with in-memory state ✅
- CLI command routing to daemon ✅

### M3

- persistent state file + lock
- tmux bootstrap

### M4

- idempotent executor + speaker-note generation
- Neovim integration pass

### M5

- nix home manager module for demo-it
- nixvim plugin that could be easily configured with nixvim
