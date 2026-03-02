# key-macro example workspace

Use this folder to test timed key playback:

```bash
demo-it ./examples/key-macro
```

This resets and creates:

- `key-macro-demo`
- `key-macro-notes`

It opens/switches to `key-macro-demo`.

`demo-it.md` includes five `demo-it` blocks:

1. open Neovim directly on the slide (`slide: slides/1-macro.1`)
2. split tmux pane to the right (`split-pane`)
3. start Neovim in the right pane (`insert-text` + `key` with `pane: last`)
4. replay timed keys in the right pane (`key-macro`, `pane: last`, per-key `delay_ms`)
5. replay a second `key-macro` step with a faster `interval_ms`
