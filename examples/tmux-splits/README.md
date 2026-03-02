# demo-it example workspace

Use this folder to test tmux bootstrap mode:

```bash
demo-it ./examples/demo
```

This resets and creates:
- `demo-demo`
- `demo-notes`

It opens/switches to `demo-demo`.

`demo-it.md` includes five `demo-it` blocks:
1. open Neovim directly on intro slide (`slide: slides/1-intro.1`)
2. split tmux pane to the right (`split-pane`)
3. open split slide (`slide: slides/2-split.2`)
4. kill extra panes and split downward (`killall-pane` + `split-pane-vertical`)
5. open wrap-up slide (`slide: slides/3-end.2`)

`demo-notes` opens Neovim with speaker notes for the current block and refreshes on `demo-it next`.
