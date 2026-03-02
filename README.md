# demo-it

`demo-it` runs transcript-driven demonstrations and presentations via CLI and nvim plugin frontends.

It provides:

- `demo-itd` daemon
- `demo-it` CLI frontend
- `DemoIt*` nvim plugin

## Dev Environment

- `flake.nix` + `devenv.nix` provide a reproducible Go shell
- `.envrc` integrates `direnv` + `devenv`
- common checks: `fmt`, `lint`, `tests`, `build`, `ci`
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
demo-it start ./examples/demo
```

Path mode is still supported and is equivalent to `demo-it start <workspace-path>`:

```bash
demo-it ./examples/tmux-splits/
```

For `examples/tmux-splits`, this creates:

- `tmux-splits-demo`
- `tmux-splits-notes`

It opens/switches to `tmux-splits-demo`; open `tmux-splits-notes` manually when needed. Workspace bootstrap also starts the daemon run context so `demo-it status` and `demo-it next` are available immediately. `demo-it start` requires `demo-it.md` in the selected workspace and exits with an error when missing.

When `demo-it` opens slides in Neovim panes, it now also runs `:DemoItPresentationEnable` (silently when available) so presentation-friendly UI defaults are applied automatically. Demo tmux sessions also start with `status off` for a cleaner stage view.

Session utilities:

- `demo-it list` lists managed demo-it workspaces with numeric indexes (demo session per workspace)
- `demo-it notes` opens the notes session for the latest workspace
- `demo-it show` opens the demo session for the latest workspace
- `demo-it kill` kills all managed demo-it tmux sessions
- `demo-it kill <index ...>` kills selected workspace sessions by index from `demo-it list`
- inside managed demo-it tmux sessions, `C-s` then `n` runs `demo-it next`, and `C-s` then `p` runs `demo-it prev`

For action-level debugging, set `DEMO_IT_DEBUG_LOG` to a file path before running commands:

```bash
DEMO_IT_DEBUG_LOG=/tmp/demo-it.log demo-it start ./examples/key-macro
DEMO_IT_DEBUG_LOG=/tmp/demo-it.log demo-it next
```

The log records tmux commands, pane targeting resolution, and key-macro step playback.

If `demo-it.md` contains `demo-it` fenced blocks, bootstrap runs the first block immediately. Steps with `slide: ...` (or `open-slide`) auto-start Neovim in the demo pane and open that slide, so `insert-text: nvim`/`key:return` is no longer required. `speaker_notes` are intended as post-action guidance before moving to the next block, and are rendered in `demo-notes` (Neovim) and refreshed as `demo-it next`/`demo-it prev` move through steps for the latest workspace. Use `clear-panes` to close extra panes gracefully by sending `C-d` until one pane remains; when the kept pane is a shell, it also clears/resets the terminal, but it skips terminal reset for Neovim panes to avoid UI redraw glitches. Use `split-pane` (right) or `split-pane-vertical` (down) for tmux layout changes while keeping the presenter pane focused; use `key-macro` for timed key playback (`interval_ms` + per-key `delay_ms`) with optional pane targeting (`pane: active|last|left|right|<index>|<pane-id>`); and use slide shorthands (`slide: ...`, `open-slide`, or `key` with `slide`) to open markdown files in a Neovim pane of the demo session.

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
