## Key macro demo

This workspace demonstrates timed key playback.

```demo-it
title: Open macro slide
slide: slides/1-macro.1
speaker_notes: |
  We open Neovim directly on the macro slide.
```

```demo-it
title: Type using key macro
actions:
  - kind: key-macro
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
  This replays a deterministic key sequence with per-key timing.
```

```demo-it
title: Add a second line with different pacing
actions:
  - kind: key-macro
    interval_ms: 40
    keys:
      - key: o
      - key: k
      - key: return
      - key: escape
speaker_notes: |
  Same action kind, faster default interval.
```
