# demo-it example workspace

Use this folder to test tmux bootstrap mode:

```bash
demo-it ./examples/demo
```

This resets and creates:
- `demo-demo`
- `demo-notes`

It opens/switches to `demo-demo`.

`demo-it.md` includes two `demo-it` blocks:
1. open Neovim (`insert-text` + `key:return`)
2. split tmux pane (`key:C-s` + `key:v`)
