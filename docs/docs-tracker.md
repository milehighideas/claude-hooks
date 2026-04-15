# docs-tracker

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/docs-tracker`)

A Go binary that gates Edit/Write operations on reading required documentation, and tracks documentation reads so the gate clears once a doc has been read.

## Overview

`docs-tracker` is a Claude Code hook with two operational modes:

1. **Enforce Mode** (PreToolUse hook): Blocks Edit/Write operations if required documentation hasn't been read during the current session.
2. **Track Mode** (PostToolUse hook): Records when documentation files are read during a session.

## Opt-in per project

The hook is **opt-in per project**. It only does anything when a project contains:

```text
<project-root>/.claude/docs-tracker.json
```

The file's presence is what enables the hook. The contents may be `{}` for the default behavior (future fields can extend config). Projects without this file see no-op behavior, which makes the hook safe to wire up globally in `~/.claude/settings.json`.

### Project root discovery

At invocation the binary walks up from the tool's `file_path` looking for a directory that contains `.claude/docs-tracker.json`. That directory is treated as the project root. If none is found anywhere on the path, the hook exits 0 and does nothing.

## Auto-discovered mappings

Once a project is opted in, the binary walks the project root looking for `CLAUDE.md` files. Each `<subdir>/CLAUDE.md` becomes a mapping: any Edit/Write of a file under `<subdir>/` requires reading that `CLAUDE.md` first.

- The **root-level** `CLAUDE.md` is ignored (Claude Code already loads it as project context).
- These directories are **skipped** during discovery: `node_modules`, `.git`, `dist`, `build`, `.next`, `.turbo`, `.vercel`, `_generated`.
- When multiple `CLAUDE.md` files apply to a path, the **most specific** (longest pattern) wins. E.g. `apps/mobile/components/CLAUDE.md` takes precedence over `apps/mobile/CLAUDE.md` for files under `components/`.

No hardcoded paths: add, move, or rename `CLAUDE.md` files freely and the mapping updates on the next invocation.

## Skip patterns

The following files are always allowed without reading documentation, even inside a mapped directory:

- `CLAUDE.md` itself
- `__tests__/` directories
- `.test.ts`, `.test.tsx` files
- `_generated/` directories
- `.d.ts` TypeScript declaration files
- `node_modules/`

## Command-line usage

```bash
docs-tracker -mode enforce < hook_input.json
docs-tracker -mode track   < hook_input.json
```

### `-mode` (required)

- `enforce`: PreToolUse mode — blocks edits if required docs haven't been read.
- `track`: PostToolUse mode — tracks documentation reads.

## Input format

Both modes read the standard Claude Code hook JSON on stdin:

```json
{
  "tool_name": "Edit",
  "tool_input": { "file_path": "/abs/path/to/file.ts" },
  "session_id": "unique-session-id"
}
```

`file_path` may be absolute or relative; relative paths are resolved against the current working directory before walking up for the project root.

## Exit codes

### Enforce mode

- `0`: Operation allowed (no config, no mapping, file skipped, or doc already read).
- `1`: System error (I/O, marshal failure).
- `2`: Operation blocked — required doc not read yet.

### Track mode

- `0`: Success (including no-op cases).
- `1`: System error.

## Blocked output

When an Edit/Write is blocked, a message is written to stderr:

```text
⚠️  PLEASE READ DOCUMENTATION FIRST

Before editing files in packages/backend, please read:
  packages/backend/CLAUDE.md

This ensures you follow project conventions and patterns.

Run: Read packages/backend/CLAUDE.md
Then retry your edit.
```

`Name` is derived from the mapped subdirectory's relative path.

## Session storage

Session data is persisted to:

```text
~/.claude/sessions/{session_id}-docs.json
```

Format:

```json
{ "docs_read": ["packages/backend/CLAUDE.md"] }
```

Doc paths are stored **relative to the project root** so enforce/track share the same keys.

## Graceful degradation

The tool fails open (allow operations) on unrecoverable issues:

- Invalid JSON input → allow.
- Missing session file → allow (first session, nothing tracked yet).
- Session file I/O error → allow.
- No project root (no `.claude/docs-tracker.json`) → allow.

System errors (exit code 1) only happen when track mode cannot write its session state.

## Wiring into Claude Code

Register the binary globally in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/bin/docs-tracker -mode enforce"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Read",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/bin/docs-tracker -mode track"
          }
        ]
      }
    ]
  }
}
```

Because the hook is a no-op without `.claude/docs-tracker.json` at the project root, the global registration is safe — it only activates inside projects that explicitly opt in.

## Building

```bash
just docs-tracker
```

Produces `bin/docs-tracker`.

## Testing

```bash
go test ./cmd/docs-tracker/...
```

Tests cover:

- Opt-in gating (no config → no-op for both modes).
- Auto-discovery (skipped dirs, longest-pattern-first ordering, root `CLAUDE.md` ignored).
- Enforce blocking/allowing behavior with skip patterns.
- Track recording, dedup, and ignore for non-Read tools / unregistered docs.
- Session persistence round-trip.

## Example walkthrough

Project layout:

```text
my-project/
├── .claude/
│   └── docs-tracker.json         # opt-in marker (may be "{}")
├── packages/backend/
│   ├── CLAUDE.md                 # required for edits under packages/backend/
│   └── utils.ts
└── apps/mobile/components/
    ├── CLAUDE.md                 # required for edits under apps/mobile/components/
    └── Button.tsx
```

1. Editing `packages/backend/utils.ts` first ⇒ blocked with a prompt to read `packages/backend/CLAUDE.md`.
2. Reading `packages/backend/CLAUDE.md` ⇒ track mode records it.
3. Editing `packages/backend/utils.ts` again ⇒ allowed.
4. Editing `apps/mobile/components/Button.tsx` ⇒ still blocked until `apps/mobile/components/CLAUDE.md` is read.
