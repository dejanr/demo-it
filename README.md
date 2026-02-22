# demo-it

`demo-it` runs transcript-driven demonstrations and presentations via CLI and Neovim frontends.

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

Use `start` to reset/create deterministic tmux sessions:

```bash
demo-it start              # bootstrap from current working directory
demo-it start ./examples/demo
```

Path mode is still supported and is equivalent to `demo-it start <workspace-path>`:

```bash
demo-it ./examples/demo
```

For `examples/demo`, this creates:
- `demo-demo`
- `demo-notes`

It opens/switches to `demo-demo`; open `demo-notes` manually when needed. Workspace bootstrap also starts the daemon run context so `demo-it status` and `demo-it next` are available immediately. `demo-it start` requires `demo-it.md` in the selected workspace and exits with an error when missing.

Session utilities:
- `demo-it list` lists managed demo-it workspaces with numeric indexes (demo session per workspace)
- `demo-it notes` opens the notes session for the latest workspace
- `demo-it kill` kills all managed demo-it tmux sessions
- `demo-it kill <index ...>` kills selected workspace sessions by index from `demo-it list`

If `demo-it.md` contains `demo-it` fenced blocks, bootstrap runs the first block immediately. Steps with `slide: ...` (or `open-slide`) auto-start Neovim in the demo pane and open that slide, so `insert-text: nvim`/`key:return` is no longer required. `speaker_notes` are intended as post-action guidance before moving to the next block, and are rendered in `demo-notes` (Neovim) and refreshed as `demo-it next`/`demo-it prev` move through steps for the latest workspace. Use `clear-panes` to close extra panes gracefully by sending `C-d` until one pane remains; when the kept pane is a shell, it also clears/resets the terminal, but it skips terminal reset for Neovim panes to avoid UI redraw glitches. Use `split-pane` (right) or `split-pane-vertical` (down) for tmux layout changes while keeping the presenter pane focused; and use slide shorthands (`slide: ...`, `open-slide`, or `key` with `slide`) to open markdown files in a Neovim pane of the demo session.

## Neovim plugin (local development)

The repository now includes a local plugin runtime:
- `plugin/demo-it.lua`
- `lua/demo-it/init.lua`

To iterate quickly from Neovim:
1. `:set rtp+=/home/dejanr/projects/demo-it`
2. `:DemoItReload`

Available commands:
- `:DemoItStart [workspace-path]` (defaults to current working directory)
- `:DemoItStatus`, `:DemoItNext`, `:DemoItPrev`
- `:DemoItRerun`, `:DemoItReloadState`
- `:DemoItJump <id|index>`
- `:DemoItFocus <present|return|none>`
- `:DemoIt <raw cli args...>`

Formatting alignment tip:
- configure Neovim formatters to call `treefmt --stdin <file> --quiet` so editor formatting matches `nix fmt`/`fmt`.
