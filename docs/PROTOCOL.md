# demo-it protocol draft (current implementation)

This document describes the transport contract between clients and `demo-itd` **as implemented today**.

## Transport

- Unix domain socket per repo/run.
- Default path pattern: `$XDG_RUNTIME_DIR/demo-it/<repo>.sock`.
- Payload format: newline-delimited JSON (NDJSON).
- One request -> one response.
- Request/response correlation via `id`.

## Envelope

### Request

```json
{
  "id": "req-123",
  "cmd": "next",
  "run_id": "demo-it-my-repo"
}
```

Notes:

- `args` is optional and command-specific when needed.

### Success response

```json
{
  "id": "req-123",
  "ok": true,
  "state": {
    "run_id": "demo-it-my-repo",
    "status": "running",
    "current_slide": 0,
    "transcript_revision": "bootstrap",
    "last_event": "interaction",
    "interaction_id": "create-tracker"
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
    "message": "invalid command: unsupported cmd \"fly\""
  }
}
```

## Commands

- `start`
  - ensure run exists in daemon memory
  - initialize seed slides for the run (current M2 behavior)
- `status`
  - return run state view
- `reload`
  - reconcile current run against provided slides/revision in-memory path
  - in current daemon service this reuses existing slides/revision
- `next`
  - advance to next interaction
  - when interactions for a slide are exhausted, move to next slide
  - at transcript end returns `ok: true` with `last_event: "end"` and `completed: true`
- `prev`
  - restore previous cursor snapshot from history
- `rerun`
  - rerun current interaction
- `jump`
  - jump to slide by `slide_id` or `slide_index`

## Command arguments

### `next` / `rerun`

No command-specific arguments currently used.

### `jump`

```json
{
  "slide_id": "wrap-up",
  "slide_index": 1
}
```

- one of `slide_id` or `slide_index` is required.

## State fields returned today

`state` currently exposes:

- `run_id`
- `status` (`idle|running|completed|failed`)
- `current_slide` (zero-based)
- `transcript_revision` (when present)
- transition metadata:
  - `last_event`
  - `interaction_id`
  - `skipped`
  - `completed`

## Transcript step format

Current `demo-it` blocks use `title`, `actions` (or `slide` shorthand), and optional `speaker_notes`.
Titles are required and must be unique within a `demo-it.md` transcript.

## Error codes

Current error code mapping:

- `invalid_request`
- `invalid_command`
- `invalid_state`
- `run_not_found`
- `internal`

## Compatibility rules

- Unknown fields in `args` are ignored.
- Unknown commands return `invalid_command`.
- Missing required envelope fields return `invalid_request`.
