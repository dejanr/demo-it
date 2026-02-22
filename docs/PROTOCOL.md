# demo-it protocol draft

This document defines the first transport contract between `demo-it` clients and `demo-itd`.

## Transport

- Unix domain socket per run/repo.
- Path pattern: `$XDG_RUNTIME_DIR/demo-it/<repo>.sock`.
- Payload format: newline-delimited JSON (NDJSON).
- Request/response correlation via `id`.

## Envelope

### Request

```json
{
  "id": "req-123",
  "cmd": "next",
  "run_id": "demo-it-my-repo",
  "client_id": "nvim-presenter-1",
  "args": {
    "focus": "present",
    "force": false
  }
}
```

### Response

```json
{
  "id": "req-123",
  "ok": true,
  "state": {
    "run_id": "demo-it-my-repo",
    "status": "running",
    "current_slide": 2,
    "current_interaction": 1,
    "default_focus": "present"
  }
}
```

### Error response

```json
{
  "id": "req-123",
  "ok": false,
  "error": {
    "code": "invalid_command",
    "message": "unsupported cmd: fly"
  }
}
```

## Commands

- `start`
  - create/attach run state
  - bootstrap presenter tmux session
  - optionally bootstrap speaker session
- `status`
  - return canonical run state
- `reload`
  - reparse transcript (`demo-it.md`)
  - refresh speaker-note artifacts
  - reconcile execution ledger by step hash
- `next`
  - advance by one interaction (or next slide boundary)
- `prev`
  - restore previous cursor snapshot from transition history
- `rerun`
  - rerun current interaction regardless of existing ledger entry
- `jump`
  - jump to slide by index or id
- `set_focus_policy`
  - set run-level default focus policy

## Shared args

### Focus policy

`focus` controls UI handoff during execution:
- `present`: focus presenter target and remain there
- `return`: focus presenter target for execution, then restore caller focus
- `none`: do not change focus

### Idempotency behavior

Each interaction carries:
- `id`
- `idempotency.key`
- step hash (derived from parsed interaction payload)
- optional `precheck`
- optional `postcheck`

Execution rules:
1. Acquire run lock.
2. Ensure environment.
3. If ledger has successful record for `(idempotency.key, step_hash)` and not forced, skip action.
4. Otherwise execute action.
5. Run `postcheck` if defined.
6. Persist ledger/history entry.

## State schema (logical)

```json
{
  "run_id": "demo-it-my-repo",
  "status": "running",
  "current_slide": 0,
  "current_interaction": -1,
  "default_focus": "present",
  "transcript_revision": "sha256:...",
  "slides": [
    {
      "id": "intro",
      "title": "Intro",
      "interactions": [
        {
          "id": "create-tracker",
          "idempotency_key": "create-tracker-v1",
          "hash": "..."
        }
      ]
    }
  ],
  "execution_ledger": {
    "create-tracker-v1": {
      "step_hash": "...",
      "status": "done"
    }
  }
}
```

## Compatibility rules

- Unknown fields in `args` must be ignored (forward-compatible).
- Unknown commands must return `invalid_command`.
- Missing required envelope fields must return `invalid_request`.
- Protocol versioning can be added later as optional `version` field.
