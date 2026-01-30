# smart-lint Documentation

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/smart-lint`)

## Overview

`smart-lint` is a Go-based intelligent project-aware code quality checker designed to work as a Claude Code hook. It automatically detects your project type and runs appropriate linters and formatters after file edits, ensuring code quality standards are maintained throughout your development process.

The tool is a modernized replacement for the bash `smart-lint.sh` script, offering improved performance, better testability, and single-binary deployment.

## What It Does

`smart-lint` provides the following functionality:

- **Automatic Project Detection**: Identifies the programming languages in your project by checking for language-specific files and configuration
- **Language-Specific Linting**: Runs the appropriate linters and formatters for each detected language
- **Project Command Priority**: Prefers project-specific lint commands (like `make lint` or `scripts/lint.sh`) over generic linters
- **Configurable File Exclusion**: Supports `.claude-hooks-ignore` file to skip files matching glob patterns
- **Claude Code Integration**: Reads JSON hook events from stdin and applies linting after file edits

## How to Use

### As a Claude Code Hook

`smart-lint` is designed to be integrated with Claude Code as a `PostToolUse` hook. When you use the Edit, Write, or MultiEdit tools in Claude Code, the hook automatically:

1. Reads the JSON event from stdin
2. Determines which file was edited
3. Detects the project type
4. Runs appropriate linters
5. Reports any issues and blocks further operations if errors are found

### Installation

1. Build the binary:

```bash
cd /Volumes/Developer/code/shared/claude-hooks/smart-lint
go build -o smart-lint
```

2. Copy to your Claude hooks directory:

```bash
cp smart-lint ~/.claude/hooks/
```

3. Configure in your `~/.claude/config.json`:

```json
{
  "hooks": {
    "PostToolUse": "~/.claude/hooks/smart-lint"
  }
}
```

### Direct Invocation

While designed for Claude Code integration, you can also test `smart-lint` directly by passing a JSON hook event via stdin:

```bash
echo '{"hook_event_name":"PostToolUse","tool_name":"Edit","tool_input":{"file_path":"./main.go"},"cwd":"."}' | ./smart-lint
```

## Input Format

`smart-lint` expects JSON input on stdin with the following structure:

```json
{
  "hook_event_name": "PostToolUse",
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "/absolute/path/to/file.go"
  },
  "cwd": "/current/working/directory"
}
```

### Field Descriptions

- **hook_event_name**: The Claude Code hook event name. Only `PostToolUse` events are processed
- **tool_name**: The tool that triggered the hook. Only `Edit`, `Write`, and `MultiEdit` are processed
- **tool_input**: Contains the `file_path` of the edited file (must be absolute path)
- **cwd**: The current working directory

## Exit Codes

`smart-lint` uses the following exit codes:

- **0**: Operation did not require linting (e.g., event type not processed, no recognized project type)
- **1**: General error occurred (invalid JSON, failed to read files, directory change errors)
- **2**: Linting issues found (blocks the operation and requires fixes)

Exit code 2 is used for both success with messages and blocking failures, with output to stderr indicating the result.

## Supported Languages

`smart-lint` automatically detects and lints the following languages:

### Go

- **Detection**: `go.mod`, `go.sum`, or 3+ `.go` files
- **Tools**: `gofmt` (auto-format), `golangci-lint` (linting)

### Python

- **Detection**: `pyproject.toml`, `setup.py`, `requirements.txt`, or 3+ `.py` files
- **Tools**: `black` (auto-format), `ruff` (with auto-fix) or `flake8` (linting)

### JavaScript/TypeScript

- **Detection**: `package.json`, `tsconfig.json`, or 3+ `.js`/`.ts`/`.jsx`/`.tsx` files
- **Tools**: `eslint` (if configured in package.json), `prettier` (if config exists)

### Rust

- **Detection**: `Cargo.toml` or 3+ `.rs` files
- **Tools**: `cargo fmt` (auto-format), `cargo clippy` (linting)

### Shell

- **Detection**: 3+ `.sh` or `.bash` files
- **Tools**: `shellcheck` (linting)

## Configuration

### Project-Level Config: .claude-hooks.json

Create a `.claude-hooks.json` file in your project root to customize lint commands:

```json
{
  "lint": "pnpm turbo lint",
  "test": "pnpm turbo test",
  "typecheck": "pnpm turbo typecheck"
}
```

When present, the `lint` command takes priority over all other linting mechanisms.

### File Ignore Patterns: .claude-hooks-ignore

Create a `.claude-hooks-ignore` file in your project root to exclude files from linting:

```text
# Comments start with #

# Exact filename matches
node_modules
dist
build

# Glob patterns
*.test.go
*_test.py
*.spec.js

# Directory patterns (everything under the directory)
vendor/**
node_modules/**
.next/**

# Multiple patterns
**/generated/*
**/mocks/*
```

#### Pattern Types Supported

- **Exact matches**: `node_modules`, `main.go`
- **Glob patterns**: `*.test.go`, `test_*.py`, `**/generated/*`
- **Directory patterns**: `vendor/**` (matches everything under vendor/)
- **Comments**: Lines starting with `#` are ignored
- **Empty lines**: Ignored

## Command Line Arguments and Flags

`smart-lint` does not accept command line arguments or flags. It is designed to be used exclusively as a Claude Code hook, reading all necessary configuration from:

1. JSON stdin input
2. `.claude-hooks.json` in project root
3. `.claude-hooks-ignore` in project root
4. Project structure and files

## Environment Variables

`smart-lint` does not use environment variables. All configuration is file-based:

- `.claude-hooks.json`: Project-level lint commands
- `.claude-hooks-ignore`: File exclusion patterns

## Project Detection Priority

When determining what linters to run, `smart-lint` follows this priority:

1. **Project Config** (`.claude-hooks.json` with `lint` command) - highest priority
2. **Project Commands** (`make lint` or `scripts/lint.sh`)
3. **Language-Specific Linters** - runs all detected languages
4. **Silent Exit** - if no project type detected

## Error Handling

### Handled Gracefully (Silent Exit)

- No recognized project type detected
- File path not provided in hook event
- File path should be skipped (matches `.claude-hooks-ignore`)
- Linter tool not installed

### Reported as Errors (Exit Code 2)

- Linting violations found
- Custom lint command failed
- Project lint command failed

### Fatal Errors (Exit Code 1)

- Invalid JSON input
- Failed to change directories
- Failed to read configuration files

## Performance Characteristics

- **Startup**: Single binary, minimal overhead
- **File Discovery**: Limited to 3 levels of directory depth for initial project detection
- **Ignored Directories**: Automatically skips `.git`, `node_modules`, `venv`, `.venv`, `target`, `dist`, `build` during file discovery
- **Early Exit**: Returns immediately if event type should not be processed

## Example Scenarios

### Scenario 1: Go Project with golangci-lint

A Go project with the following structure:

```text
project/
├── go.mod
├── main.go
├── pkg/
│   └── lib.go
└── .claude-hooks-ignore
```

When `main.go` is edited:

1. Detects Go project (via `go.mod`)
2. Runs `gofmt` on all `.go` files
3. Runs `golangci-lint run`
4. Reports any issues or confirms success

### Scenario 2: JavaScript Project with Custom Lint Command

Configuration in `.claude-hooks.json`:

```json
{
  "lint": "pnpm turbo lint"
}
```

When any file is edited:

1. Runs custom command: `pnpm turbo lint`
2. Blocks further operations if command fails
3. Allows continuation if successful

### Scenario 3: Multi-Language Project with Exclusions

`.claude-hooks-ignore`:

```text
*.test.ts
node_modules/**
dist/**
```

When a file is edited:

1. Detected languages: JavaScript and Python
2. Tests files are skipped
3. Runs `eslint` and `prettier` for JavaScript
4. Runs `black` and `ruff` for Python

## Architecture Details

The implementation is organized into distinct functions:

- **Input Handling**: `parseHookEvent()`, `shouldProcess()`
- **Project Detection**: `detectProjectType()`, `findProjectRoot()`
- **Configuration**: `loadProjectConfig()`, `loadIgnorePatterns()`
- **File Discovery**: `findFiles()`, `shouldSkipFile()`
- **Language Linters**: `lintGo()`, `lintPython()`, `lintJavaScript()`, `lintRust()`, `lintShell()`
- **Command Execution**: `runLanguageLinter()`, `tryProjectCommand()`, `runCustomCommand()`
- **Error Collection**: `ErrorCollector` type and `exitWithResult()` function

## Testing

`smart-lint` includes comprehensive unit tests covering:

- JSON parsing and event filtering
- Project type detection
- Ignore pattern matching and file skipping
- File discovery with ignore patterns
- Project root detection
- Error collection and reporting
- Integration scenarios

Run tests with:

```bash
go test -v
```

## Limitations and Known Behaviors

1. **Linter Availability**: If a tool isn't installed, it's silently skipped. For example, if `golangci-lint` is not installed, only `gofmt` runs.

2. **Project Type Detection**: Detection is based on file markers checked up to 3 directory levels. Projects without standard markers may not be detected.

3. **Formatting Auto-Apply**: Some linters (gofmt, prettier, black) automatically fix formatting issues. If a linter reports issues but cannot auto-fix them, the operation is blocked.

4. **Single File Scope**: The hook processes events per file edit but runs project-wide linters when available (e.g., `make lint` runs on entire project).

5. **Exit Code Semantics**: Exit codes 0 and 2 both represent successful processing; the difference is whether linting issues were found (code 2 blocks further operations).

## Troubleshooting

### Linter Not Running

Check if:

- The linter tool is installed (`which eslint`, `which gofmt`, etc.)
- File extension is recognized by the linter
- File path is not excluded by `.claude-hooks-ignore`

### Unexpected Files Being Linted

Check:

- `.claude-hooks-ignore` patterns are correct
- File extension matches the intended language
- No glob pattern is too broad

### Project Type Not Detected

Ensure your project has one of the following markers:

- Go: `go.mod`, `go.sum`, or 3+ `.go` files
- Python: `pyproject.toml`, `setup.py`, `requirements.txt`, or 3+ `.py` files
- JavaScript: `package.json`, `tsconfig.json`, or 3+ `.js`/`.ts`/`.jsx`/`.tsx` files
- Rust: `Cargo.toml` or 3+ `.rs` files
- Shell: 3+ `.sh` or `.bash` files

## Future Enhancements

Potential improvements for future versions:

- Debug mode with verbose output
- Performance metrics and timing information
- More granular control over which linters run
- Support for additional languages (Nix, Tilt)
- Custom linter command chains
- Cache management for faster repeated linting
