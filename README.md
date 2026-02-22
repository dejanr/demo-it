# demo-it

`demo-it` is a Go-first backend for transcript-driven live demos.

It provides:
- `demo-itd` daemon: canonical run state + tmux orchestration + idempotent execution
- `demo-it` CLI: command client for terminal and Neovim plugin
- protocol + state machine boundaries with tests

Neovim is intentionally a thin client:
- keymaps and command forwarding
- slide/speaker rendering
- no canonical execution state in the editor

## Project layout

- `cmd/demo-it` CLI entrypoint
- `cmd/demo-itd` daemon entrypoint
- `internal/protocol` command envelope, args, validation
- `internal/session` run state, interactions, transitions
- `docs/PROTOCOL.md` wire protocol draft
- `docs/CLI-PLAN.md` CLI-focused implementation plan

## Status

M2 foundation in place:
- protocol draft + validation tests
- core state machine + transition tests
- daemon socket server with in-memory run service
- CLI command routing over Unix socket transport

## Dev environment

- `flake.nix` + `devenv.nix` provide a reproducible Go shell
- `.envrc` integrates `direnv` + `devenv`
- common checks: `fmt`, `lint`, `tests`, `build`, `ci`
- shared formatting config: `treefmt.toml` (used by both `nix fmt` and `fmt`)
- `devenv up` runs `demo-itd` with auto-reload via `.air.toml`

## Quick tmux workspace bootstrap

Use path mode to reset/create deterministic tmux sessions for a workspace:

```bash
demo-it ./examples/demo
```

For `examples/demo`, this creates:
- `demo-demo`
- `demo-notes`

It opens/switches to `demo-demo`; open `demo-notes` manually when needed. Workspace bootstrap also starts the daemon run context so `demo-it status` and `demo-it next` are available immediately.

Session utilities:
- `demo-it list` lists managed demo-it tmux sessions with numeric indexes
- `demo-it kill` kills all managed demo-it tmux sessions
- `demo-it kill <index ...>` kills selected sessions by index from `demo-it list`

If `demo-it.md` contains `demo-it` fenced blocks, bootstrap replays the first block `actions` into `demo-demo` (for example: `insert-text` + `key` to open Neovim). `speaker_notes` are intended as post-action guidance before moving to the next block. `key` actions can express tmux chord sequences as separate actions (for example `C-s` then `v`, depending on your tmux bindings).

## Neovim plugin (local development)

The repository now includes a local plugin runtime:
- `plugin/demo-it.lua`
- `lua/demo-it/init.lua`

To iterate quickly from Neovim:
1. `:set rtp+=/home/dejanr/projects/demo-it`
2. `:DemoItReload`

Available commands:
- `:DemoItStart`, `:DemoItStatus`, `:DemoItNext`, `:DemoItPrev`
- `:DemoItRerun`, `:DemoItReloadState`
- `:DemoItJump <id|index>`
- `:DemoItFocus <present|return|none>`
- `:DemoIt <raw cli args...>`

Formatting alignment tip:
- configure Neovim formatters to call `treefmt --stdin <file> --quiet` so editor formatting matches `nix fmt`/`fmt`.
