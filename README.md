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
