# docs-tracker

A unified Go binary that replaces two Python hooks for tracking and enforcing documentation reads in Claude Code sessions.

## Overview

This tool provides two operational modes:

1. **Enforce Mode** (PreToolUse hook): Blocks Edit/Write operations if required documentation hasn't been read
2. **Track Mode** (PostToolUse hook): Records when documentation files are read during a session

## Features

- **Session-based tracking**: Maintains state per Claude Code session
- **Pattern-based enforcement**: Maps file paths to required documentation
- **Intelligent skipping**: Automatically allows test files, generated files, and documentation files themselves
- **Clear error messages**: Provides helpful guidance when documentation needs to be read

## Building

```bash
go build -o docs-tracker
```

## Usage

### Enforce Mode (PreToolUse)

```bash
docs-tracker --mode=enforce < hook_input.json
```

Exit codes:

- `0`: Operation allowed
- `2`: Operation blocked (documentation not read)

### Track Mode (PostToolUse)

```bash
docs-tracker --mode=track < hook_input.json
```

Exit codes:

- `0`: Success

## Configuration

### Tracked Documentation Files

The following documentation files are tracked:

- `packages/backend/CLAUDE.md` - Convex backend
- `apps/mobile/components/CLAUDE.md` - Mobile components
- `apps/mobile/app/CLAUDE.md` - Mobile app routing

### Directory Mappings

Files in these directories require reading the corresponding documentation:

- `packages/backend/**` → `packages/backend/CLAUDE.md`
- `apps/mobile/components/**` → `apps/mobile/components/CLAUDE.md`
- `apps/mobile/app/**` → `apps/mobile/app/CLAUDE.md`

### Skipped Patterns

The following files are automatically allowed without documentation:

- `CLAUDE.md` files themselves
- `__tests__/` directories
- `.test.ts` and `.test.tsx` files
- `_generated/` directories
- `.d.ts` TypeScript declaration files
- `node_modules/` directories

## Input Format

The tool expects JSON input on stdin with the following structure:

```json
{
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "packages/backend/foo.ts"
  },
  "session_id": "unique-session-id"
}
```

## Session Storage

Session data is stored in:

```text
~/.claude/sessions/{session_id}-docs.json
```

Format:

```json
{
  "docs_read": [
    "packages/backend/CLAUDE.md",
    "apps/mobile/components/CLAUDE.md"
  ]
}
```

## Testing

Run the comprehensive test suite:

```bash
go test -v
```

The test suite includes:

- Enforce mode tests (blocking and allowing operations)
- Track mode tests (session persistence)
- Utility function tests
- Integration tests

## Migration from Python

This binary replaces:

- `enforce-docs-read.py` → `docs-tracker --mode=enforce`
- `track-docs-read.py` → `docs-tracker --mode=track`

Session files are compatible with the Python versions, so no migration is needed.

## Example Workflow

1. User reads backend documentation:

   ```bash
   echo '{"tool_name":"Read","tool_input":{"file_path":"packages/backend/CLAUDE.md"},"session_id":"abc123"}' \
     | docs-tracker --mode=track
   ```

2. User edits backend file (allowed):

   ```bash
   echo '{"tool_name":"Edit","tool_input":{"file_path":"packages/backend/utils.ts"},"session_id":"abc123"}' \
     | docs-tracker --mode=enforce
   # Exit code: 0
   ```

3. User tries to edit mobile components without reading docs (blocked):
   ```bash
   echo '{"tool_name":"Edit","tool_input":{"file_path":"apps/mobile/components/Button.tsx"},"session_id":"abc123"}' \
     | docs-tracker --mode=enforce
   # Exit code: 2, with helpful error message
   ```

## License

MIT
