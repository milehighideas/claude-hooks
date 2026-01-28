# Block Infrastructure Hook

A unified Claude Code hook that protects critical infrastructure files from modification by AI agents.

## Features

This single binary replaces two Python hooks:

- `block-infrastructure-bash-edits.py` - Blocks Bash commands that edit infrastructure
- `block-infrastructure-edits.py` - Blocks Edit/Write/NotebookEdit tools on infrastructure

## What It Protects

### Global Infrastructure

- `~/.claude/hooks/` - All hook scripts
- `~/.claude/CLAUDE.md` - Global instructions
- `~/.claude/settings.json` - Claude settings
- `~/.claude/hooks/ast_utils.py` - Hook utilities
- `~/.claude/hooks/ast-parser.js` - Hook utilities
- `~/.claude/hooks/srp_validators.py` - Hook utilities

### Project-Level Infrastructure

- `.claude-hooks-config.sh` - Project hook configuration
- `.claude-hooks-ignore` - Hook ignore patterns
- `.claude/hooks/*.py` - Project hook scripts
- `.claude/hooks/*.sh` - Project hook scripts
- `.claude/hooks/*.js` - Project hook scripts

## How It Works

The hook receives JSON input from Claude Code and checks:

1. **For Bash commands**: Extracts file paths from commands like:
   - Redirects: `cat > file`, `echo >> file`
   - In-place edits: `sed -i`, `awk -i`
   - Move/copy: `mv`, `cp` to protected files
   - Text editors: `vim`, `nano`, `emacs`
   - Heredocs: `cat << EOF > file`

2. **For Edit tools**: Checks the `file_path` or `notebook_path` parameter

If a protected file is detected:

- Prints an error message to stderr
- Exits with code 2 (blocks the operation)

Otherwise:

- Exits with code 0 (allows the operation)

## Installation

1. Build the binary:

   ```bash
   cd /Volumes/Developer/code/shared/claude-hooks/block-infrastructure
   go build -o block-infrastructure
   ```

2. Configure Claude Code to use this hook as a PreToolUse hook for both Bash and Edit tools.

## Testing

Run the comprehensive test suite:

```bash
go test -v
```

The tests cover:

- File path extraction from various Bash commands
- Protection of global infrastructure files
- Protection of project-level infrastructure patterns
- Proper handling of both Bash and Edit tools
- Path normalization with symlinks
- JSON parsing

## Development

### Project Structure

- `main.go` - Main hook logic
- `main_test.go` - Comprehensive test suite
- `go.mod` - Go module definition

### Key Functions

- `handleBashTool()` - Process Bash commands
- `handleEditTool()` - Process Edit/Write/NotebookEdit operations
- `extractFilePathsFromCommand()` - Extract file paths from shell commands
- `isProtectedFile()` - Check if a file is protected
- `normalizePath()` - Normalize paths with symlink resolution

## Exit Codes

- `0` - Allow (operation is safe)
- `2` - Block (operation targets protected infrastructure)

## Why This Matters

This hook ensures AI agents cannot:

- Modify their own instructions or constraints
- Disable or bypass quality control hooks
- Change project hook configuration
- Edit critical infrastructure files

This protection maintains the integrity of the AI development environment.
