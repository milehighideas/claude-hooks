# smart-test

A Claude Code PostToolUse hook that automatically runs tests after editing files.

## Features

- Automatically runs tests after Edit/Write/MultiEdit tool usage
- Detects project type (Go, Python, JavaScript/TypeScript, Rust, Shell)
- Supports project-specific test commands (`make test`, `scripts/test.sh`)
- Language-specific test runners:
  - **Go**: `go test -race ./...` (race detection enabled by default)
  - **Python**: `pytest` or `python -m unittest discover`
  - **JavaScript/TypeScript**: `npm test`
  - **Rust**: `cargo test`
  - **Shell**: Looks for corresponding `*_test.sh` files
- Respects `.claude-hooks-ignore` file to skip specific files/directories
- Exit code 2 blocks Claude from continuing if tests fail

## Installation

```bash
cd /Volumes/Developer/code/shared/claude-hooks/smart-test
go build -o smart-test
```

## Configuration

### Environment Variables

- `CLAUDE_HOOKS_TEST_ON_EDIT` (default: `true`): Enable/disable test-on-edit
- `CLAUDE_HOOKS_ENABLE_RACE` (default: `true`): Enable/disable Go race detector

### Ignore Patterns

Create a `.claude-hooks-ignore` file in your project root:

```text
# Ignore generated files
*_generated.go
*.pb.go

# Ignore vendor directories
vendor/**
node_modules/**

# Ignore specific files
config.yaml
```

## Project Commands

The hook will prefer project-specific test commands:

1. `make test` (if Makefile exists with test target)
2. `scripts/test.sh` or `scripts/test` (if exists)
3. Language-specific test runners (fallback)

## Usage

Add to your Claude Code config:

```json
{
  "hooks": [
    {
      "event": "PostToolUse",
      "command": "/Volumes/Developer/code/shared/claude-hooks/smart-test/smart-test"
    }
  ]
}
```

## Testing

```bash
go test -v
```

## How It Works

1. Reads hook event JSON from stdin
2. Checks if testing is enabled (`CLAUDE_HOOKS_TEST_ON_EDIT`)
3. Only processes `PostToolUse` events for Edit/Write/MultiEdit tools
4. Checks if file should be ignored (`.claude-hooks-ignore`)
5. Detects project type based on files and directories
6. Runs appropriate test command:
   - Try `make test` first
   - Try `scripts/test.sh` second
   - Fall back to language-specific test runners
7. Exits with code 2 if tests fail (blocks Claude) or pass (allows continuation)

## Example Output

Success:

```text
✅ All tests passed. Continue with your task.
```

Failure:

```text
❌ go test failed
--- FAIL: TestFoo (0.00s)
    foo_test.go:10: expected 42, got 43

❌ Tests failed with 1 error(s)
⛔ BLOCKING: Fix ALL test failures above before continuing
```

## Development

The hook follows the same architecture as `smart-lint`:

- Minimal dependencies (only Go stdlib)
- Fast execution (exits early when not applicable)
- Clear error messages
- Comprehensive test coverage
