# track-edited-files

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/track-edited-files`)

## Description

`track-edited-files` is a Claude hook that automatically tracks which files are edited during a Claude coding session. It categorizes edited files into two groups: source files and test files, and persists this data to a session-specific JSON file in the user's home directory.

This tool is designed to support test enforcement and audit capabilities by maintaining a record of:

- **Source files** - Production code files that were modified
- **Test files** - Test files that were created or modified

The tracker uses intelligent filtering to only monitor relevant source code files and automatically excludes configuration files, generated code, and other non-trackable files.

## Usage

`track-edited-files` is designed to be used as a **Claude hook** that integrates with the Claude development environment. It is triggered automatically when files are edited during a session, not as a standalone CLI tool.

### Integration

The tool reads JSON input from stdin with the following structure:

```json
{
  "session_id": "unique-session-identifier",
  "tool_input": {
    "file_path": "/path/to/edited/file.ts"
  }
}
```

When invoked, the tool:

1. Reads the file path from the input
2. Determines if the file should be tracked based on filtering rules
3. Categorizes the file as either a source file or test file
4. Appends the file to the appropriate session tracking file
5. Exits with status 0 (non-blocking operation)

## Command Line Arguments

The tool does not accept command-line arguments. All input is provided via JSON on stdin.

## Environment Variables

The tool uses the following environment variable:

- `$HOME` - Determines the location of the session tracking directory. Session files are stored at `~/.claude/sessions/{session_id}.json`

## File Tracking Rules

### Valid Source Files

The tool tracks TypeScript and JavaScript files with these extensions:

- `.ts`
- `.tsx`
- `.js`
- `.jsx`

Files must be located in one of these directories to be tracked:

- `packages/backend/convex/` - Convex backend functions
- `apps/mobile/` - React Native mobile app code

### Files Excluded from Tracking

The following files are never tracked:

**By pattern:**

- Generated code (`_generated/` directory)
- Test files (`.test.ts`, `.test.tsx`, `__tests__/` directory)
- Type declarations (`.d.ts`)
- Special files: `schema.ts`, `index.ts`, `Types.ts`, `Constants.ts`
- Configuration files (`.config.`, `jest.setup`, `vitest.config`, `.eslintrc`, `.prettierrc`)

**By extension:**

- `.md` (markdown)
- `.json` (JSON)
- `.css` (CSS)
- `.scss` (SCSS)

**Mobile app config files:**

- `metro.config.js`
- `babel.config.js`
- `app.config.ts`

**Files in unsupported directories:**

- `apps/web/` (web app)
- `packages/backend/src/` (non-convex backend)
- Any other location outside the tracked paths

## Exit Codes

The tool always exits with status **0**, regardless of success or failure. This is intentional - the tool operates as a non-blocking hook that never interrupts the development workflow.

Exit behavior:

- **0** - Always (success, invalid input, errors, etc.)

The tool silently handles all error conditions without propagating them to the caller.

## Session Data Storage

Session data is stored as JSON in `~/.claude/sessions/{session_id}.json`:

```json
{
  "source_files": [
    "/project/packages/backend/convex/users.ts",
    "/project/apps/mobile/src/components/Button.tsx"
  ],
  "test_files": ["/project/packages/backend/convex/users.test.ts"]
}
```

The tool:

- Creates the `~/.claude/sessions/` directory if it doesn't exist
- Appends new files to existing session data (no duplicates)
- Preserves previously tracked files across multiple invocations
- Creates the directory with permissions `0755` and files with permissions `0644`

## Example Usage

### Example 1: Tracking a Convex backend function

When a user edits `/project/packages/backend/convex/users.ts`:

```json
{
  "session_id": "abc-123-def",
  "tool_input": {
    "file_path": "/project/packages/backend/convex/users.ts"
  }
}
```

Result: File is added to `~/.claude/sessions/abc-123-def.json` under `source_files`.

### Example 2: Tracking a test file

When a user creates `/project/packages/backend/convex/users.test.ts`:

```json
{
  "session_id": "abc-123-def",
  "tool_input": {
    "file_path": "/project/packages/backend/convex/users.test.ts"
  }
}
```

Result: File is added to `~/.claude/sessions/abc-123-def.json` under `test_files`.

### Example 3: Ignored file (configuration)

When a user edits `/project/apps/mobile/package.json`:

```json
{
  "session_id": "abc-123-def",
  "tool_input": {
    "file_path": "/project/apps/mobile/package.json"
  }
}
```

Result: File is ignored (not added to tracking) because it's a `.json` configuration file.

### Example 4: Ignored file (wrong directory)

When a user edits `/project/apps/web/src/Button.tsx`:

```json
{
  "session_id": "abc-123-def",
  "tool_input": {
    "file_path": "/project/apps/web/src/Button.tsx"
  }
}
```

Result: File is ignored because it's in the `apps/web/` directory, which is not tracked.

## Deduplication

The tool prevents duplicate entries in session tracking. If a file has already been recorded in a session, it will not be added again, even if the tracking hook is triggered multiple times for the same file.
