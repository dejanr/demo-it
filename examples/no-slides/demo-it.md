## No slides demo

This workspace demonstrates action-only transcripts.

```demo-it
title: Print intro line
auto_slide_in_ms: 2000
actions:
  - kind: insert-text
    text: clear
  - kind: key
    key: return
speaker_notes: |
  Clear the pane and auto slide in 2s
  It auto-advances after 2 seconds.
```

```demo-it
title: Show current workspace
auto_slide_in_ms: 1000
actions:
  - kind: insert-text
    text: echo lets count to 3
  - kind: key
    key: return
```

```demo-it
title: Count 1
auto_slide_in_ms: 1000
actions:
  - kind: insert-text
    text: echo 1
  - kind: key
    key: return
```

```demo-it
title: Count 2
auto_slide_in_ms: 1000
actions:
  - kind: insert-text
    text: echo 2
  - kind: key
    key: return
```

```demo-it
title: Count 3
auto_slide_in_ms: 1000
actions:
  - kind: insert-text
    text: echo 3
  - kind: key
    key: return
```

```demo-it
title: Exit the demo
auto_slide_in_ms: 2000
actions:
  - kind: insert-text
    text: exit
  - kind: key
    key: return
speaker_notes: |
  Exit the demo
```
