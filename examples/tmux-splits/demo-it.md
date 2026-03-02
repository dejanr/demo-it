## Intro

Welcome to demo-it example.

```demo-it
title: Open Neovim with intro slide
slide: slides/1-intro.1
speaker_notes: |
  Now that Neovim is open, I can show you X, Y, and Z.
  Then we proceed with the next demo-it block.
```

Next step is preserving the slide, and just opening split pane on the right

```demo-it
title: Split tmux pane
actions:
  - kind: split-pane
    direction: right
speaker_notes: |
  Now we split the tmux window and keep Neovim in one pane.
```

```demo-it
title: Open split layout slide
slide: slides/2-split.2
speaker_notes: |
  Switch the editor to the split-layout slide content.
```

```demo-it
title: Kill all extra panes and split vertically instead
actions:
  - kind: killall-pane
  - kind: split-pane-vertical
speaker_notes: |
  We start this slide with a single pane before layout changes.
  We can also split downward with the dedicated vertical action.
```

```demo-it
title: Open wrap-up slide
slide: slides/3-end.2
actions:
  - kind: killall-pane
speaker_notes: |
  Transition to the final slide for closing remarks.
```

## Wrap up

Done.
