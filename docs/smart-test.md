# smart-test

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/smart-test`)

A Claude Code PostToolUse hook that automatically runs tests after editing files.

## Overview

`smart-test` is a Claude Code hook that triggers automated test execution whenever you modify code files using the Edit, Write, or MultiEdit tools. It intelligently detects your project type and runs the appropriate test command, blocking Claude from continuing if tests fail to ensure code quality.

## Features

- **Automatic test execution** after Edit/Write/MultiEdit tool usage
- **Multi-language support** for Go, Python, JavaScript/TypeScript, Rust, and Shell
- **Intelligent project detection** based on configuration files and source code
- **Project-level test commands** with support for `make test` and `scripts/test.sh`
- **Language-specific test runners**:
  - Go: `go test -race ./...` (race detection enabled by default)
  - Python: `pytest` or `python -m unittest discover`
  - JavaScript/TypeScript: `npm test`
  - Rust: `cargo test`
  - Shell: Looks for corresponding `*_test.sh` files
- **Selective file ignoring** via `.claude-hooks-ignore` file
- **Exit code blocking** prevents Claude from continuing if tests fail
- **Minimal dependencies** using only Go standard library

## Usage

### Installation

Build the binary:

```bash
cd /Volumes/Developer/code/shared/claude-hooks/smart-test
go build -o smart-test
```

### Enabling the Hook

Add to your Claude Code configuration (typically in `.claude-code/config.json` or Claude Code settings):

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

The hook operates as a Claude Code hook that reads JSON events from stdin and processes them automatically.

## Command Line Interface

The `smart-test` binary does not accept command-line arguments. It is designed to be used exclusively as a Claude Code PostToolUse hook.

### Input (stdin)

The hook expects JSON input from Claude Code in the following format:

```json
{
  "hook_event_name": "PostToolUse",
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "/path/to/file.go"
  },
  "cwd": "/path/to"
}
```

### Output

The hook writes status messages and test output to stderr with emoji indicators:

- `✅ All tests passed. Continue with your task.` - Tests passed successfully
- `❌ [test name] failed` - Individual test failure
- `⛔ BLOCKING: Fix ALL test failures above before continuing` - Multiple failures with blocking notice

## Environment Variables

### CLAUDE_HOOKS_TEST_ON_EDIT

Controls whether test execution is enabled.

- **Default**: `true` (enabled)
- **Values**: `true`, `1` (enabled) or `false`, `0` (disabled)
- **Example**: `CLAUDE_HOOKS_TEST_ON_EDIT=false smart-test`

When disabled, the hook exits immediately without processing.

### CLAUDE_HOOKS_ENABLE_RACE

Controls whether Go race detector is enabled in `go test` commands.

- **Default**: `true` (enabled)
- **Values**: `true`, `1` (enabled) or `false`, `0` (disabled)
- **Example**: `CLAUDE_HOOKS_ENABLE_RACE=false smart-test`

Only affects Go projects. When enabled, runs `go test -race ./...`. When disabled, runs `go test ./...`.

## Configuration

### Project Configuration (.claude-hooks.json)

Create a `.claude-hooks.json` file in your project root for custom test commands:

```json
{
  "test": "pnpm turbo test",
  "lint": "pnpm turbo lint",
  "typecheck": "pnpm turbo typecheck"
}
```

The `test` field specifies a custom command to run instead of auto-detection. When present, this takes precedence over all other test discovery methods.

### Ignore Patterns (.claude-hooks-ignore)

Create a `.claude-hooks-ignore` file in your project root to skip tests for specific files or directories:

```text
# Ignore generated files
*_generated.go
*.pb.go
*.generated.ts

# Ignore vendor and dependency directories
vendor/**
node_modules/**
dist/**

# Ignore specific files
config.example.yaml
secrets.local.json

# Comments and blank lines are ignored
```

Supports the following pattern types:

- **Exact matches**: `config.yaml` (matches filename anywhere in path or exact path)
- **Glob patterns**: `*_generated.go` (standard glob syntax)
- **Directory patterns**: `vendor/**` (matches entire directory and contents)

## Exit Codes

- **0**: Hook disabled via environment variable
- **1**: Error during execution (e.g., failed to parse input or change directory)
- **2**: Tests completed (both success and failure):
  - If all tests passed: Allows Claude to continue
  - If any tests failed: Blocks Claude from continuing

Exit code 2 is used for both success and failure to provide blocking behavior in Claude Code.

## How It Works

The hook executes the following workflow:

1. **Initialization**: Checks if testing is enabled via `CLAUDE_HOOKS_TEST_ON_EDIT`
2. **Event parsing**: Reads and validates JSON hook event from stdin
3. **Event filtering**: Only processes PostToolUse events for Edit, Write, or MultiEdit tools
4. **File extraction**: Gets the file path that was edited
5. **Directory setup**: Changes to the directory containing the edited file
6. **Ignore checking**: Skips if file matches patterns in `.claude-hooks-ignore`
7. **Configuration loading**: Looks for `.claude-hooks.json` in project root
8. **Project detection**: Identifies project languages and structure
9. **Test execution**: Runs tests using one of the following strategies:
   - **Custom command** from `.claude-hooks.json` (if present)
   - **Project commands**: `make test` or `scripts/test.sh` (if present)
   - **Language-specific runners** (fallback)
10. **Result reporting**: Outputs test results and exits with code 2

## Test Execution Strategy

The hook attempts test execution in this priority order:

1. **Custom project config** (`make test` → `scripts/test.sh` → language fallback)
   - Custom test command from `.claude-hooks.json`
2. **Project-level commands**
   - `make test` (if Makefile with test target exists)
   - `scripts/test.sh` or `scripts/test` (if executable exists)
3. **Language-specific runners**
   - Go: `go test -race ./...`
   - Python: `pytest` or `python -m unittest discover`
   - JavaScript/TypeScript: `npm test`
   - Rust: `cargo test`
   - Shell: Runs `*_test.sh` files matching edited script

## Project Type Detection

The hook detects project types by checking for configuration and source files:

- **Go**: `go.mod`, `go.sum`, or `.go` files
- **Python**: `pyproject.toml`, `setup.py`, `requirements.txt`, or `.py` files
- **JavaScript/TypeScript**: `package.json`, `tsconfig.json`, or `.js`/`.ts`/`.jsx`/`.tsx` files
- **Rust**: `Cargo.toml` or `.rs` files
- **Shell**: `.sh` or `.bash` files

Multi-language projects are supported, and the hook will run tests for all detected languages.

## Example Usage

### Go Project with Race Detection

Edit a Go file:

```bash
# Input from Claude Code (stdin)
{
  "hook_event_name": "PostToolUse",
  "tool_name": "Edit",
  "tool_input": {"file_path": "/myproject/cmd/main.go"},
  "cwd": "/myproject"
}

# Output
✅ All tests passed. Continue with your task.
# Exit code: 2
```

### Test Failure Example

If tests fail:

```bash
# Stderr output
❌ go test failed
--- FAIL: TestCalculate (0.00s)
    calculate_test.go:15: expected 5, got 4

❌ Tests failed with 1 error(s)
⛔ BLOCKING: Fix ALL test failures above before continuing
# Exit code: 2 (blocks Claude)
```

### Ignoring Generated Files

With `.claude-hooks-ignore`:

```text
*_generated.go
*.pb.go
```

Editing these files will skip test execution.

### Custom Test Command

With `.claude-hooks.json`:

```json
{
  "test": "pnpm turbo test --filter=@myapp/web"
}
```

This custom command runs instead of `npm test` or other auto-detected commands.

## Development

### Building from Source

```bash
cd /Volumes/Developer/code/shared/claude-hooks/smart-test
go build -o smart-test
```

### Running Tests

```bash
cd /Volumes/Developer/code/shared/claude-hooks/smart-test
go test -v
```

Test coverage includes:

- Hook event parsing and validation
- Project type detection for all supported languages
- File ignore pattern matching (exact, glob, and directory patterns)
- Environment variable configuration
- Project root discovery
- Error handling and exit codes

### Architecture

The implementation uses only Go standard library with no external dependencies:

- `encoding/json` for hook event parsing
- `os/exec` for running test commands
- `filepath` and `strings` for file pattern matching
- `bufio` for reading ignore patterns

## Troubleshooting

### Hook Not Triggering

**Problem**: Tests don't run when editing files

**Solutions**:

- Verify `CLAUDE_HOOKS_TEST_ON_EDIT` is not set to `false`
- Check Claude Code configuration includes the hook
- Ensure the hook binary is executable: `chmod +x smart-test`
- Verify the hook path is correct in configuration

### Tests Not Found

**Problem**: Hook runs but no tests execute

**Solutions**:

- Verify your project has test files or a test runner
- Check project type detection by looking at project root markers (e.g., `go.mod`, `package.json`)
- Confirm test commands exist: `make test`, `scripts/test.sh`, or language-specific runners
- Review `.claude-hooks-ignore` to ensure test files aren't excluded

### Race Detector Too Strict (Go)

**Problem**: Go race detector fails in CI or specific environments

**Solution**: Disable race detection:

```bash
CLAUDE_HOOKS_ENABLE_RACE=false smart-test
```

### Large Test Suites Slow Down Editing

**Problem**: Hook execution slows Claude when working on large projects

**Solutions**:

- Create a custom test command in `.claude-hooks.json` that runs only relevant tests
- Use `.claude-hooks-ignore` to skip certain files
- Set `CLAUDE_HOOKS_TEST_ON_EDIT=false` to disable testing when not needed
- Run specific test suites instead of all tests via custom commands

## Related Tools

This hook is part of the claude-hooks suite:

- `smart-lint` - Intelligent linting on file edits
- `convex-gen` - Convex API code generation
- `enforce-tests-on-commit` - Git pre-commit test enforcement
