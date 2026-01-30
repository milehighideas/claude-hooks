# docs-tracker

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/docs-tracker`)

A unified Go binary that replaces two Python hooks for tracking and enforcing documentation reads in Claude Code sessions.

## Overview

`docs-tracker` is a Claude Code hook that ensures developers read required documentation before editing code in specific directories. It provides two operational modes:

1. **Enforce Mode** (PreToolUse hook): Blocks Edit/Write operations if required documentation hasn't been read during the current session
2. **Track Mode** (PostToolUse hook): Records when documentation files are read during a session

## How It's Used

### As a Claude Code Hook

This tool is designed to be integrated with Claude Code as a hook system:

- **PreToolUse Hook**: Invoked before Edit and Write tools execute. Blocks operations if required documentation hasn't been read.
- **PostToolUse Hook**: Invoked after the Read tool executes. Tracks which documentation files have been read.

The hooks receive JSON input via stdin containing tool information and session data, and communicate results via exit codes and stderr messages.

### Command Line Usage

```bash
docs-tracker --mode=enforce < hook_input.json
docs-tracker --mode=track < hook_input.json
```

## Command Line Arguments

### `--mode` (required)

Specifies the operation mode. Must be one of:

- `enforce`: PreToolUse mode - blocks edits if required docs haven't been read
- `track`: PostToolUse mode - tracks documentation reads

Example:

```bash
docs-tracker --mode=enforce < input.json
docs-tracker --mode=track < input.json
```

## Environment Variables

None. The tool uses:

- `$HOME/.claude/sessions/{session_id}-docs.json` for session storage
- Falls back to current directory if `$HOME` cannot be determined

## Exit Codes

### Enforce Mode

- `0`: Operation allowed (documentation was read or file doesn't require documentation)
- `1`: System error (invalid input, file I/O error)
- `2`: Operation blocked (required documentation has not been read)

### Track Mode

- `0`: Success
- `1`: System error (invalid input, file I/O error)

## Input Format

The tool expects JSON input on stdin:

```json
{
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "packages/backend/foo.ts"
  },
  "session_id": "unique-session-id"
}
```

### Input Fields

- `tool_name` (string): Name of the tool being used (Edit, Write, Read, etc.)
- `tool_input` (object): Tool-specific parameters, must include `file_path`
- `session_id` (string): Unique identifier for the Claude Code session

## Enforce Mode Behavior

Enforce mode checks Edit and Write operations:

1. If the tool is not Edit or Write, the operation is allowed
2. If the file path doesn't require documentation (test files, generated files, etc.), the operation is allowed
3. If the file path requires documentation:
   - Checks if that documentation has been read in the current session
   - If yes: operation is allowed (exit code 0)
   - If no: operation is blocked (exit code 2) with an error message

### Blocked Operations Output

When an operation is blocked, the tool writes a message to stderr:

```bash
⚠️  PLEASE READ DOCUMENTATION FIRST

Before editing files in Mobile components, please read:
  apps/mobile/components/CLAUDE.md

This ensures you follow project conventions and patterns.

Run: Read apps/mobile/components/CLAUDE.md
Then retry your edit.
```

## Track Mode Behavior

Track mode monitors Read operations:

1. If the tool is not Read, nothing is tracked
2. If the file path is not a tracked documentation file, nothing is tracked
3. If the file is a tracked documentation file:
   - Records that file in the session's doc tracking data
   - Prevents duplicates (same file won't be recorded twice)
   - Returns exit code 0

Tracked documentation files:

- `packages/backend/CLAUDE.md`
- `apps/mobile/components/CLAUDE.md`
- `apps/mobile/app/CLAUDE.md`

## Configuration

Configuration is hardcoded in the binary and maps file patterns to required documentation:

### Directory Mappings

Files in these directories require reading the corresponding documentation:

| Directory Pattern         | Required Documentation             |
| ------------------------- | ---------------------------------- |
| `packages/backend/`       | `packages/backend/CLAUDE.md`       |
| `apps/mobile/components/` | `apps/mobile/components/CLAUDE.md` |
| `apps/mobile/app/`        | `apps/mobile/app/CLAUDE.md`        |

### Skip Patterns

The following files are automatically allowed without reading documentation:

- `CLAUDE.md` - documentation files themselves
- `__tests__/` - test directories
- `.test.ts`, `.test.tsx` - test files
- `_generated/` - generated code directories
- `.d.ts` - TypeScript declaration files
- `node_modules/` - node modules directories

## Session Storage

Session data is persisted to:

```text
~/.claude/sessions/{session_id}-docs.json
```

### Session File Format

```json
{
  "docs_read": [
    "packages/backend/CLAUDE.md",
    "apps/mobile/components/CLAUDE.md"
  ]
}
```

The directory is created automatically if it doesn't exist (with mode 0755).

## Example Usage

### Scenario 1: User reads backend documentation, then edits a backend file

Step 1 - Read documentation (Track mode):

```bash
echo '{
  "tool_name":"Read",
  "tool_input":{"file_path":"packages/backend/CLAUDE.md"},
  "session_id":"abc123"
}' | docs-tracker --mode=track

# Exit code: 0
# Session file created with packages/backend/CLAUDE.md tracked
```

Step 2 - Edit backend file (Enforce mode):

```bash
echo '{
  "tool_name":"Edit",
  "tool_input":{"file_path":"packages/backend/utils.ts"},
  "session_id":"abc123"
}' | docs-tracker --mode=enforce

# Exit code: 0 (allowed - documentation was read)
```

### Scenario 2: User tries to edit file without reading documentation

```bash
echo '{
  "tool_name":"Edit",
  "tool_input":{"file_path":"apps/mobile/components/Button.tsx"},
  "session_id":"abc123"
}' | docs-tracker --mode=enforce

# Exit code: 2 (blocked)
# Stderr output explains which documentation must be read
```

### Scenario 3: Edit test files without documentation

Test files are automatically allowed:

```bash
echo '{
  "tool_name":"Edit",
  "tool_input":{"file_path":"packages/backend/utils.test.ts"},
  "session_id":"abc123"
}' | docs-tracker --mode=enforce

# Exit code: 0 (allowed - test files skip documentation requirement)
```

## Building

Build the binary:

```bash
go build -o docs-tracker
```

## Testing

Run the comprehensive test suite:

```bash
go test -v
```

Test coverage includes:

- **Enforce mode tests**: Verifying blocking and allowing operations
- **Track mode tests**: Verifying session persistence and doc tracking
- **Utility function tests**: Testing helper functions
- **File pattern tests**: Testing skip patterns and directory matching
- **Session persistence tests**: Testing file storage and loading

## Migration from Python

This binary replaces two Python hooks:

- `enforce-docs-read.py` → `docs-tracker --mode=enforce`
- `track-docs-read.py` → `docs-tracker --mode=track`

Session files are compatible with the Python versions (same JSON format), so no migration is needed.

## Error Handling

### Graceful Degradation

The tool is designed to fail open (allow operations) when errors occur:

- Invalid JSON input: operation is allowed (assumed external system error)
- Missing session file: operation is allowed (first session, no docs tracked yet)
- Session file I/O errors: operation is allowed (assumes system issue, not user issue)

### Reported Errors

System errors (exit code 1) are reported to stderr:

- Invalid JSON format
- Filesystem errors (directory creation, file writing)
- JSON marshal/unmarshal errors

## Implementation Details

The tool is implemented in Go with the following key components:

- **Mode handling**: Separate code paths for enforce and track modes
- **Session persistence**: JSON-based storage in user home directory
- **File pattern matching**: Simple string contains matching for directory patterns
- **Exit code signaling**: Using custom ExitError type for proper exit code propagation

See `/Volumes/Developer/code/shared/claude-hooks/docs-tracker/main.go` for implementation.
