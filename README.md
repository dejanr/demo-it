# demo-it

`demo-it` runs transcript-driven demonstrations and presentations via CLI and nvim plugin frontends.

It provides:

- `demo-itd` daemon
- `demo-it` CLI frontend
- `DemoIt*` nvim plugin

## Dev Environment

- `flake.nix` + `devenv.nix` provide a reproducible Go shell
- `.envrc` integrates `direnv` + `devenv`
- common checks: `fmt`, `lint`, `tests`, `tests-e2e`, `build`, `ci`
- shared formatting config: `treefmt.toml` (used by both `nix fmt` and `fmt`)
- `devenv up` runs `demo-itd` with auto-reload via `.air.toml`

## Home Manager module (user daemon)

`demo-it` exports a Home Manager module as `homeManagerModules.default` (alias: `homeManagerModules.demo-it`) so you can run `demo-itd` as a user service.

1. Add the flake input:

```nix
# flake.nix
inputs.demo-it.url = "github:dejanr/demo-it";
```

1. Wire the module into Home Manager.

If you use Home Manager through NixOS/nix-darwin, add it to `sharedModules`:

```nix
home-manager.sharedModules = [
  inputs.demo-it.homeManagerModules.default
];
```

If you use standalone Home Manager, import it in your home config:

```nix
{
  imports = [ inputs.demo-it.homeManagerModules.default ];
}
```

1. Enable the service in your home configuration:

```nix
{
  services.demo-it = {
    enable = true;
  };
}
```

When enabled, the module:

- installs `demo-it` and `demo-itd`
- starts `systemd --user` service `demo-itd`
- exports `DEMO_IT_SOCKET` (by default) so CLI commands target the same daemon

## Nixvim plugin package

The flake also exports a Neovim plugin package as `packages.<system>.demo-it-nvim`.

Example usage from another flake:

```nix
extraPlugins = [
  inputs.demo-it.packages.${pkgs.stdenv.hostPlatform.system}.demo-it-nvim
];
```

This is the recommended way to share the plugin with nixvim/Home Manager setups.

## Quick tmux workspace bootstrap

Use `start` to reset/create deterministic tmux sessions:

```bash
demo-it start              # bootstrap from current working directory
demo-it start ./examples/tmux-splits
demo-it start ./examples/no-slides
```

Path mode is still supported and is equivalent to `demo-it start <workspace-path>`:

```bash
demo-it ./examples/tmux-splits/
```

For `examples/tmux-splits`, this creates:

- `tmux-splits-demo`
- `tmux-splits-notes`

It opens/switches to `tmux-splits-demo`; open `tmux-splits-notes` manually when needed. Workspace bootstrap also starts the daemon run context so `demo-it status` and `demo-it next` are available immediately. In single-workspace mode, `demo-it start` first kills previously managed demo-it tmux sessions, then creates the new workspace sessions. `demo-it start` requires `demo-it.md` in the selected workspace and exits with an error when missing. Slide assets are optional: action-only transcripts (for shell demos) also work, see `examples/no-slides`.

When `demo-it` opens slides in Neovim panes, it now also runs `:DemoItPresentationEnable` (silently when available) so presentation-friendly UI defaults are applied automatically. Demo tmux sessions also start with `status off` for a cleaner stage view.

Session utilities:

- `demo-it list` lists managed demo-it workspaces with numeric indexes (demo session per workspace)
- `demo-it notes` opens the notes session for the latest workspace
- `demo-it show` opens the demo session for the latest workspace
- `demo-it trace-next` traces active demo-pane output while executing `next` and writes `.demo-it/traces/*.log` plus normalized `.txt` snapshots
- `demo-it kill` kills all managed demo-it tmux sessions
- `demo-it kill <index ...>` kills selected workspace sessions by index from `demo-it list`
- inside managed demo-it tmux sessions, `C-s` then `n` runs `demo-it next`, and `C-s` then `p` runs `demo-it prev`

For action-level debugging, set `DEMO_IT_DEBUG_LOG` to a file path before running commands:

```bash
DEMO_IT_DEBUG_LOG=/tmp/demo-it.log demo-it start ./examples/key-macro
DEMO_IT_DEBUG_LOG=/tmp/demo-it.log demo-it next
```

Set `DEMO_IT_DEBUG_LOG` on `start` so tmux keybindings inherit the same debug log path.
The log records tmux commands, pane targeting resolution, and key-macro step playback.

For pane-level tracing, run:

```bash
demo-it trace-next
```

This taps the active pane in the latest demo session using `tmux pipe-pane`, runs `next`, writes raw output to `<workspace>/.demo-it/traces/<timestamp>-next-pane-<id>.log`, and writes a normalized textual snapshot to the same path with `.txt` extension.

Snapshot coverage for this flow is available via:

```bash
tests-e2e
```

For local development, you can force tmux keybindings to use a specific binary via `DEMO_IT_PATH` on `start`:

```bash
DEMO_IT_PATH=./bin/demo-it demo-it start ./examples/key-macro
```

On `start`, `demo-it` also writes tmux global environment values (`DEMO_IT_PATH`, `DEMO_IT_RUN_ID`, `DEMO_IT_SOCKET`) so prefix bindings can execute with matching runtime context.
If `DEMO_IT_PATH` is empty, keybindings use `demo-it` from `PATH`.

Minimal tmux prefix bindings (for custom tmux configs), including inline error messages:

```tmux
bind -N "demo-it next" Space run-shell -b 'out=$("${DEMO_IT_PATH:-demo-it}" next 2>&1); code=$?; [ "$code" -eq 0 ] || tmux display-message "demo-it next failed: $(printf "%s" "$out" | tr "\n" " ")"'
bind -N "demo-it prev" BSpace run-shell -b 'out=$("${DEMO_IT_PATH:-demo-it}" prev 2>&1); code=$?; [ "$code" -eq 0 ] || tmux display-message "demo-it prev failed: $(printf "%s" "$out" | tr "\n" " ")"'
```

If `demo-it.md` contains `demo-it` fenced blocks, bootstrap runs the first block immediately. Slides are optional: blocks can be pure tmux/shell actions, or they can use `slide: ...` / `open-slide` (which auto-start Neovim in the demo pane and open that slide, so `insert-text: nvim`/`key:return` is no longer required). `speaker_notes` are intended as post-action guidance before moving to the next block, and are rendered in `demo-notes` (Neovim scratch buffer) and refreshed as `demo-it next`/`demo-it prev` move through steps for the latest workspace without writing workspace notes files. Use `auto_slide_in_ms` inside a block to auto-trigger `next` after the given delay (with a step guard so manual navigation cancels the pending auto-advance). Use `killall-pane` to kill all extra panes and keep only the initial pane (lowest pane index). Use `split-pane` (right) or `split-pane-vertical` (down) for tmux layout changes while keeping the presenter pane focused; use `kill-pane` to close the latest pane (highest pane index) in the target session; use `key-macro` for timed key playback (`interval_ms` + per-key `delay_ms`) with optional pane targeting (`pane: active|last|left|right|<index>|<pane-id>`); and use slide shorthands (`slide: ...`, `open-slide`, or `key` with `slide`) to open markdown files in a Neovim pane of the demo session.

Example `key-macro` action:

```yaml
- kind: key-macro
  pane: last
  interval_ms: 80
  keys:
    - key: i
    - key: h
    - key: return
      delay_ms: 250
```

## Neovim plugin (local development)

The repository now includes a local plugin runtime:

- `plugin/demo-it.lua`
- `lua/demo-it/init.lua`

To iterate quickly from Neovim:

1. `:set rtp+=/home/dejanr/projects/demo-it`
2. `:lua require("demo-it").setup()`
3. `:DemoItReload`

Available commands:

- `:DemoItStart [workspace-path]` (defaults to current working directory; enables presentation mode on success)
- `:DemoItStatus`, `:DemoItNext`, `:DemoItPrev`
- `:DemoItRerun`, `:DemoItReloadState`
- `:DemoItJump <id|index>`
- `:DemoItFocus <present|return|none>`
- `:DemoItPresentationEnable`, `:DemoItPresentationDisable`, `:DemoItPresentationToggle`
- `:DemoItPreview`, `:DemoItPreviewStop`
- `:DemoIt <raw cli args...>`

`DemoItPresentation` mode borrows the simple parts of zen/editing plugins: it reduces UI noise (line numbers/signs/status UI), adds light reading padding (`winbar`, `scrolloff`, `sidescrolloff`, extra fold column), turns on wrapping for easier reading, suppresses markdown diagnostics while active, and switches Markdown conceal between rendered (`Normal`) and raw (`Insert`) editing modes.

`DemoItPreview` prefers `:LivePreview start` when available and falls back to `:MarkdownPreview`.

If Neovim runs inside Kitty with remote control enabled (`KITTY_LISTEN_ON`), presentation mode also bumps terminal font size and restores it when disabled.

You can configure presentation font sizing when setting up the plugin:

```lua
require("demo-it").setup({
  presentation = {
    disable_markdown_diagnostics = true, -- default: true
    font = {
      neovide_scale = 1.25, -- multiply current neovide_scale_factor
      kitty_delta = 3, -- kitty @ set-font-size +3 while enabled
    },
  },
})
```

Formatting alignment tip:

- configure Neovim formatters to call `treefmt --stdin <file> --quiet` so editor formatting matches `nix fmt`/`fmt`.

## Attribution

This project is strongly inspired by Howard Abrams' [`demo-it`](https://github.com/howardabrams/demo-it).
I watched his presentation a long time ago, and it stayed with me.
Many ideas here are a direct nod to his work.

## License

This project is licensed under the MIT License (see `LICENSE`).
