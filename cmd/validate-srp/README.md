# validate-srp

Go binary that validates Single Responsibility Principle (SRP) compliance for TypeScript/TSX files.

## Overview

This tool validates code against architectural rules:

- No direct Convex imports outside data-layer/backend/convex
- No state management in screens
- One export per CRUD file
- File size limits (warnings)
- Type exports must be in types/ folders
- No mixed concerns (data + UI + state in same file)

## Building

```bash
go build -o validate-srp
```

## Usage

### Standalone Mode (CLI)

Check all TypeScript files in a directory:

```bash
# Check current directory
validate-srp --path .

# Check specific directory
validate-srp --path apps/mobile

# Check with verbose output (shows passed files)
validate-srp --path . --verbose

# Check a single file
validate-srp --file src/components/MyComponent.tsx
```

### Claude Hook Mode

The binary also works as a Claude hook, reading JSON from stdin:

```bash
echo '{"tool_name":"Write","tool_input":{"file_path":"/app/Component.tsx","content":"..."}}' | CLAUDE_HOOKS_AST_VALIDATION=true ./validate-srp
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `-path <dir>` | Directory to recursively check |
| `-file <file>` | Single file to check |
| `-v, -verbose` | Show verbose output (including passed files) |
| `-h, -help` | Show help message |

## Exit Codes

- `0` - No violations (allow operation)
- `1` - Error running checks
- `2` - SRP violation detected (block operation)

## Checks Performed

1. **Direct Convex Imports**: Prevents direct Convex imports outside allowed directories (data-layer, backend, convex, scripts, providers)

2. **State in Screens**: Prevents state management (useState, useReducer, useContext) in screen files

3. **Multiple Exports**: Enforces one export per CRUD component file

4. **File Size**: Warns about files exceeding size limits:
   - Screens: 100 lines
   - Hooks: 150 lines
   - Components: 200 lines

5. **Type Exports Location**: Enforces type exports in types/ folders

6. **Mixed Concerns**: Warns when a file mixes data fetching, UI components, and state management

## Shell Aliases

Add to your `.aliases` or `.bashrc`:

```bash
# SRP validator (standalone)
alias srp='/path/to/validate-srp'
alias srp-here='srp --path .'
alias srp-mobile='srp --path apps/mobile'
alias srp-verbose='srp --path . --verbose'
```

## Environment Variables

- `CLAUDE_HOOKS_AST_VALIDATION=true` - Enable validation in hook mode (opt-in only)
- `CLAUDE_HOOKS_AST_VALIDATION=false` - Disable validation

## Testing

Run the test suite:

```bash
go test -v
```
