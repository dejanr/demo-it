## Shell-only demo

This workspace demonstrates action-only transcripts.

```demo-it
title: Print intro line
actions:
  - kind: insert-text
    text: clear
  - kind: key
    key: return
speaker_notes: |
  This block runs pure shell commands in the active pane.
```

```demo-it
title: Show current workspace
actions:
  - kind: insert-text
    text: pwd
  - kind: key
    key: return
speaker_notes: |
  No slide files are required for this flow.
```

```demo-it
title: Exit the demo
actions:
  - kind: insert-text
    text: exit
  - kind: key
    key: return
speaker_notes: |
  Exit the demo
```
