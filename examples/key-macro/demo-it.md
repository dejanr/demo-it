## Key macro demo

This workspace demonstrates timed key playback.

```demo-it
title: Open macro slide
slide: slides/1-macro.1
speaker_notes: |
  We open Neovim directly on the macro slide.
```

```demo-it
title: Split right pane for macro target
actions:
  - kind: split-pane
    direction: right
speaker_notes: |
  We keep the presenter focus on the left pane and use the right pane as macro target.
```

```demo-it
title: Start Neovim in right pane
actions:
  - kind: insert-text
    pane: last
    text: nvim
  - kind: key
    pane: last
    key: return
speaker_notes: |
  This starts a dedicated editor in the right pane where key macros will be replayed.
```

```demo-it
title: Type using key macro in right pane
actions:
  - kind: key-macro
    pane: last
    interval_ms: 65
    keys:
      - key: i
      - key: h
      - key: e
      - key: l
      - key: l
      - key: o
      - key: "-"
      - key: d
      - key: e
      - key: m
      - key: o
      - key: return
        delay_ms: 300
      - key: escape
speaker_notes: |
  This replays a deterministic key sequence with per-key timing into the right pane.
```

```demo-it
title: Add a second line with different pacing
actions:
  - kind: key-macro
    pane: last
    interval_ms: 40
    keys:
      - key: o
      - key: k
      - key: return
      - key: escape
speaker_notes: |
  Same action kind, faster default interval.
```
