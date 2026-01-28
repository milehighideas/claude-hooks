# smart-lint

A Go-based intelligent project-aware code quality checker for Claude Code hooks.

## Overview

`smart-lint` automatically detects your project type and runs appropriate linters/formatters after file edits in Claude Code. It's a direct port of the bash `smart-lint.sh` script with improved performance and maintainability.

## Features

- **Automatic Project Detection**: Detects Go, Python, JavaScript/TypeScript, Rust, and Shell projects
- **Language-Specific Linting**: Runs appropriate linters for each language:
  - Go: `gofmt`, `golangci-lint`
  - Python: `black`, `ruff`/`flake8`
  - JavaScript/TypeScript: `eslint`, `prettier`
  - Rust: `cargo fmt`, `cargo clippy`
  - Shell: `shellcheck`
- **Project Command Support**: Prefers project-specific commands (`make lint`, `scripts/lint.sh`)
- **File Ignore Patterns**: Supports `.claude-hooks-ignore` file with glob patterns
- **Claude Code Integration**: Reads JSON hook events from stdin

## Installation

```bash
cd /Volumes/Developer/code/shared/claude-hooks/smart-lint
go build -o smart-lint
```

Install to your Claude Code hooks directory:

```bash
cp smart-lint ~/.claude/hooks/
```

Configure as a Claude Code hook in your `~/.claude/config.json`:

```json
{
  "hooks": {
    "PostToolUse": "~/.claude/hooks/smart-lint"
  }
}
```

## Usage

`smart-lint` is designed to be used as a Claude Code hook. It reads JSON from stdin:

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

### Exit Codes

- `0`: Allow operation to continue (no issues found, or not an edit operation)
- `1`: General error (missing dependencies, invalid input)
- `2`: Block operation (linting issues found)

## Configuration

### .claude-hooks-ignore

Create a `.claude-hooks-ignore` file in your project root to skip files:

```text
# Skip test files
*.test.go
*_test.py

# Skip directories
vendor/**
node_modules/**

# Skip specific files
generated.go
```

Supports:

- Exact matches: `file.txt`
- Glob patterns: `*.test.go`, `test_*.py`
- Directory patterns: `vendor/**`
- Comments: Lines starting with `#`

### Project Commands

If your project has a `Makefile` with a `lint` target or a `scripts/lint.sh` script, `smart-lint` will use those instead of language-specific linters.

## Development

### Running Tests

```bash
go test -v
```

### Building

```bash
go build -o smart-lint
```

## Architecture

The code is organized into clear sections:

1. **JSON Input Parsing**: Reads and validates Claude Code hook events
2. **Event Filtering**: Only processes PostToolUse events for Edit/Write/MultiEdit
3. **Project Detection**: Auto-detects languages from config files and extensions
4. **File Ignore Logic**: Loads and applies ignore patterns
5. **Linter Execution**: Runs language-specific linters via `exec.Command`
6. **Project Commands**: Checks for and prefers project-specific lint commands
7. **Error Collection**: Aggregates all errors and exits with appropriate code

## Differences from Bash Version

- Single binary, no external dependencies beyond linters
- Faster startup time
- Easier to test and maintain
- Simplified configuration (no environment variables yet)
- Core functionality matches bash version

## Future Enhancements

Potential additions:

- Environment variable configuration
- Debug mode support
- Timing/performance metrics
- More granular control over which linters run
- Support for additional languages (Nix, Tilt)

## License

MIT
