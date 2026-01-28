# block-infrastructure Hook

**Repository:** [https://github.com/milehighideas/block-infrastructure](https://github.com/milehighideas/block-infrastructure)


## Overview

`block-infrastructure` is a Claude Code hook that prevents AI agents from modifying critical infrastructure files and configuration settings. It acts as a security boundary to ensure agents cannot circumvent quality controls, modify their own behavioral constraints, or alter user configuration files.

The hook intercepts file modification operations and blocks attempts to edit protected infrastructure files while allowing normal project development to proceed unimpeded.

## Purpose

This hook protects the integrity of:

- **Global user configuration** (~/.claude/CLAUDE.md, ~/.claude/settings.json)
- **Global hook scripts** (hooks/*.py, *.sh, *.js files in ~/.claude/hooks/)
- **Project-level hook infrastructure** (.claude-hooks-config.sh, .claude-hooks-ignore)
- **Project hook scripts** (.claude/hooks/*.py, *.sh, *.js within the current project)

## How It Works

The hook monitors two categories of tool operations:

1. **Bash Commands**: Detects and blocks shell commands that attempt to modify protected files using:
   - Redirects: `>`, `>>`
   - In-place edits: `sed -i`, `awk -i`
   - File operations: `mv`, `cp`
   - Text editors: `vim`, `vi`, `nano`, `emacs`
   - Heredocs: `cat << EOF > file`

2. **File Edit Tools**: Blocks direct file modifications via Claude Code's Edit, Write, and NotebookEdit tools

## Usage

This hook is automatically invoked by Claude Code's hook system. No manual invocation is required.

### Integration

The hook is registered in Claude Code's configuration and runs automatically when:

1. A `Bash` tool is executed with a command containing file modification operations
2. An `Edit` tool is used to modify a file
3. A `Write` tool is used to create or overwrite a file
4. A `NotebookEdit` tool is used to modify a Jupyter notebook

### Hook Input Format

The hook receives JSON-formatted input via stdin:

```json
{
  "tool_name": "Bash",
  "tool_input": {
    "command": "echo 'test' > ~/.claude/CLAUDE.md"
  },
  "cwd": "/path/to/project"
}
```

Fields:
- `tool_name`: The Claude Code tool being executed (Bash, Edit, Write, NotebookEdit)
- `tool_input`: Parameters for the tool (command string or file paths)
- `cwd`: Current working directory context

## Command Line Arguments

This is not a command-line tool; it runs as a hook. However, it can be invoked manually for testing:

```bash
# Send JSON input via stdin
echo '{"tool_name":"Bash","tool_input":{"command":"echo test > ~/.claude/CLAUDE.md"},"cwd":"/tmp"}' | ./block-infrastructure
```

The hook reads input from stdin and does not accept command-line arguments.

## Environment Variables

No environment variables are used by this hook.

## Exit Codes

- **0**: Operation allowed (file is not protected or tool is not monitored)
- **2**: Operation blocked (attempted to modify a protected file)

Exit code 2 signals Claude Code to halt the attempted operation and display the blocking message to the user.

## Protected Files and Patterns

### Global Infrastructure (Home Directory)

- `~/.claude/CLAUDE.md` - Global user instructions
- `~/.claude/settings.json` - Claude Code settings
- `~/.claude/hooks/*.py` - Python hook scripts
- `~/.claude/hooks/*.sh` - Shell hook scripts
- `~/.claude/hooks/*.js` - JavaScript hook scripts

### Project-Level Infrastructure

Relative to the current project directory:

- `.claude-hooks-config.sh` - Project hook configuration
- `.claude-hooks-ignore` - Project hook ignore patterns
- `.claude/hooks/*.py` - Project Python hooks
- `.claude/hooks/*.sh` - Project shell hooks
- `.claude/hooks/*.js` - Project JavaScript hooks

## Error Handling

When a protected file modification is detected:

1. The hook writes a detailed error message to stderr
2. Exits with code 2 to signal the block
3. The error message includes:
   - What command or operation was attempted
   - Which file was protected
   - The reason it's protected
   - Guidance on what to do instead

Example blocking message:

```bash
❌ BLOCKED: Cannot modify infrastructure file via Bash

Command: echo 'test' > ~/.claude/CLAUDE.md

File: /Users/user/.claude/CLAUDE.md
Reason: ~/.claude/CLAUDE.md is protected global configuration

Infrastructure files are protected from modification via shell commands.

If you need to modify this file:
1. Ask the user to make the change manually
2. Or ask the user to temporarily disable this hook

Protected file categories:
- Hook scripts (~/.claude/hooks/*.py, *.sh, *.js)
- Global instructions (~/.claude/CLAUDE.md)
- Settings (~/.claude/settings.json)
- Project hook configuration (.claude-hooks-config.sh, .claude-hooks-ignore)

This protection ensures agents cannot circumvent quality controls.
```

## Example Usage

### Blocked: Attempting to modify CLAUDE.md via Bash

```bash
echo "new instructions" > ~/.claude/CLAUDE.md
```

Result: Exit code 2 with blocking message

### Blocked: Attempting to edit hook script

```bash
vim ~/.claude/hooks/test.py
```

Result: Exit code 2 with blocking message

### Blocked: Attempting to use Write tool on global config

```python
# Claude Code: Write to ~/.claude/CLAUDE.md
```

Result: Exit code 2 with blocking message

### Blocked: Modifying project hook configuration

```bash
sed -i 's/old/new/' .claude-hooks-config.sh
```

Result: Exit code 2 with blocking message

### Allowed: Regular project file operations

```bash
echo "code" > src/main.go
vim src/main.go
sed -i 's/old/new/' src/config.json
```

Result: Exit code 0, operations proceed normally

### Allowed: Reading protected files

```bash
cat ~/.claude/CLAUDE.md
grep pattern ~/.claude/hooks/test.py
```

Result: Exit code 0 (read operations are not blocked, only modifications)

## File Path Resolution

The hook handles path normalization including:

- Tilde expansion: `~` → user's home directory
- Relative paths: Resolved relative to current working directory
- Symlinks: Evaluated to their canonical paths
- Path traversal: Detects and blocks `..` escapes from protected directories

## Security Considerations

This hook provides defense-in-depth against agent circumvention of:

1. **Behavioral constraints** - Prevents agents from modifying CLAUDE.md to change their own instructions
2. **Hook manipulation** - Prevents agents from disabling or replacing hooks
3. **Configuration hijacking** - Prevents agents from modifying settings
4. **Recursive escape** - Works in conjunction with other hooks to maintain integrity

The protection applies equally to all agents and cannot be bypassed by:

- Using different tools or commands
- Modifying file extensions
- Using redirects or pipe chains
- Copying then editing
- In-place editor operations

## Testing

The hook includes comprehensive test coverage:

```bash
go test ./block-infrastructure
```

Test cases verify:

- File path extraction from various bash patterns
- Protected file detection for global and project-level files
- Path normalization with tilde expansion and symlinks
- Blocking behavior for Bash and Edit tools
- Allowance of non-edit operations
- JSON input parsing and routing
