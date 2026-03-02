# key-macro example workspace

Use this folder to test timed key playback:

```bash
demo-it ./examples/key-macro
```

This resets and creates:

- `key-macro-demo`
- `key-macro-notes`

It opens/switches to `key-macro-demo`.

`demo-it.md` includes three `demo-it` blocks:

1. open Neovim directly on the slide (`slide: slides/1-macro.1`)
2. replay timed keys with `key-macro` and per-key `delay_ms`
3. replay a second `key-macro` step with a faster `interval_ms`
