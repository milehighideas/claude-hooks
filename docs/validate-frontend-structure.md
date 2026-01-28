# validate-frontend-structure

**Repository:** [https://github.com/milehighideas/validate-frontend-structure](https://github.com/milehighideas/validate-frontend-structure)


A Claude Code PreToolUse hook that enforces consistent frontend folder structure in React/Next.js projects.

## Overview

The `validate-frontend-structure` tool validates that frontend features follow a standardized CRUD-based folder structure. It intercepts structure-modifying operations (file writes, edits, and bash commands) and blocks them if they would violate the architecture constraints.

This hook is designed to work with the `frontend-architecture` Claude skill and enforces the recommended folder organization pattern across your components directory.

## Usage

### As a Claude Hook (Default)

This tool operates as a **PreToolUse hook** in Claude Code, meaning it validates operations before they execute:

1. Enable the hook in your project by setting the environment variable
2. When you attempt structure-modifying operations in the components directory, the hook validates them
3. If violations are detected, the operation is blocked and you receive detailed error messages

### As a CLI Tool

You can also build and run the tool manually:

```bash
# Build the binary
go build -o validate-frontend-structure

# Run it directly (accepts JSON on stdin)
echo '{"tool_name": "Write", "tool_input": {"file_path": "/project/components/feature.tsx"}}' | ./validate-frontend-structure
```

## Configuration

### Environment Variables

**`CLAUDE_HOOKS_AST_VALIDATION`** (required to enable)

- **Type**: String boolean
- **Default**: Not set (hook is disabled)
- **Values**: `"true"` to enable, any other value or unset disables
- **Purpose**: Opt-in flag that must be explicitly set to activate validation

The hook is **opt-in only**. Without this environment variable, all operations are allowed.

### Project Configuration File

Create a `.claude-hooks-config.sh` file in your project root to set environment variables:

```bash
#!/bin/bash
export CLAUDE_HOOKS_AST_VALIDATION=true
```

The hook automatically sources this file on startup if it exists.

## Command Line Arguments and Flags

This tool does not accept command-line arguments or flags. It operates entirely through:

1. **Standard input**: Expects JSON-formatted tool use data
2. **Environment variables**: Configuration via `CLAUDE_HOOKS_AST_VALIDATION`
3. **File system**: Discovers project root by searching for `package.json`

### JSON Input Format

When invoked as a hook, the tool receives JSON on stdin with this structure:

```json
{
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/path/to/components/feature.tsx",
    ...
  }
}
```

## Exit Codes

- **`0`** - Operation allowed
  - Validation passed (structure compliant)
  - Hook is disabled (`CLAUDE_HOOKS_AST_VALIDATION != "true"`)
  - Operation is not structure-modifying
  - Cannot parse input or find project root (fails gracefully)

- **`2`** - Operation blocked
  - Frontend structure validation failed
  - Detailed error messages written to stderr
  - User must fix architecture issues before retrying

## Validated Operations

The hook only validates operations that modify component structure. It checks:

### Monitored Tool Types

- **Write**: Creating `.ts` or `.tsx` files in `/components/` directories
- **Edit**: Modifying `.ts` or `.tsx` files in `/components/` directories
- **Bash**: Structure-creating commands (`mkdir`, `touch`, `mv`, `cp`) in `/components/`

### Ignored Operations

- Read, Glob, Grep, and other non-modifying operations
- Delete operations (`rm` commands are not validated)
- Files outside `/components/` directories
- Non-TypeScript files (`.json`, `.css`, etc.)

## Supported Directory Structures

The tool automatically detects and validates these project layouts:

1. **Monorepo with web app**
   ```text
   project/
   └── apps/web/components/
   ```

2. **Single app project**
   ```text
   project/
   └── components/
   ```

## Required Structure

Each feature in `components/routes/` and `components/shared/` must follow this exact structure:

```typescript
feature-name/
├── index.ts              # Main barrel export (required)
├── create/               # CRUD operations
│   ├── index.ts          # Required
│   └── .gitkeep          # Required
├── read/
│   ├── index.ts
│   └── .gitkeep
├── update/
│   ├── index.ts
│   └── .gitkeep
├── delete/
│   ├── index.ts
│   └── .gitkeep
├── hooks/                # Custom React hooks
│   ├── index.ts
│   └── .gitkeep
├── screens/              # Full page/screen components
│   ├── index.ts
│   └── .gitkeep
├── types/                # TypeScript type definitions
│   ├── index.ts
│   └── .gitkeep
└── utils/                # Utility functions
    ├── index.ts
    └── .gitkeep
```

### Required Elements

- **8 folders**: `create`, `read`, `update`, `delete`, `hooks`, `screens`, `types`, `utils`
- **index.ts files**: One in each folder (barrel export) + one in feature root (main export)
- **.gitkeep files**: One in each of the 8 folders (ensures empty folders are tracked by git)
- **No loose components**: All `.tsx`/`.ts` files must be inside feature folders

## Validation Rules

1. **Folder presence**: All required folders must exist
2. **Barrel exports**: Each folder and the feature root must have `index.ts`
3. **Git tracking**: Each folder must have `.gitkeep` to ensure it's tracked
4. **No loose files**: `.tsx` and `.ts` files cannot exist directly in:
   - `components/` root
   - Feature folders (must be in CRUD/hooks/screens/types/utils subdirectories)
5. **No loose components in root**: The `components/` directory itself cannot contain `.tsx` or `.ts` files except `index.ts`

## Error Messages and Recovery

### Common Issues and Solutions

**Missing required folder**
```text
Missing required folder: feature-name/create/
```
Fix: Create the folder with `mkdir -p components/routes/feature-name/create`

**Missing barrel export**
```bash
Missing barrel export file: feature-name/create/index.ts
```
Fix: Create the file with content like `export * from './Component';`

**Missing .gitkeep**
```text
Missing Git tracking file: feature-name/create/.gitkeep
```
Fix: Create with `touch components/routes/feature-name/create/.gitkeep`

**Loose component file**
```bash
Loose component file (must be in feature folder): components/LooseComponent.tsx
```
Fix: Move file into appropriate feature folder under the CRUD structure

**Full validation failure** returns:
```bash
BLOCKED: Frontend structure validation failed

The following issues were found with your frontend architecture:

  - [list of specific issues]

Required structure for each feature in components/routes/ and components/shared/:
  - All CRUD folders: create/, read/, update/, delete/
  - Other folders: hooks/, screens/, types/, utils/
  - Each folder must have: index.ts and .gitkeep
  - Main feature folder must have: index.ts

No loose .tsx files allowed in components/ root - use feature folders!

To fix:
1. Create missing folders and files
2. Move loose components to appropriate feature folders
3. Follow the frontend-architecture skill guidelines

See: ~/.claude/skills/frontend-architecture/SKILL.md
```

## Example Usage

### Enable and Validate

In your project root, create `.claude-hooks-config.sh`:

```bash
export CLAUDE_HOOKS_AST_VALIDATION=true
```

When you now try to create files in components, the hook validates them:

```bash
# This will be validated when using Claude Code Write tool
components/routes/product/create/CreateProduct.tsx
```

If the folder structure is invalid, you'll see validation errors that explain what's missing.

### Fixing Structure

If you have loose components:

```bash
# Before (invalid)
components/
├── UserForm.tsx          # ❌ Loose file

# After (valid)
components/
└── routes/
    └── user/
        ├── create/
        │   ├── UserForm.tsx
        │   ├── index.ts      # export { UserForm } from './UserForm'
        │   └── .gitkeep
        ├── read/
        │   ├── index.ts
        │   └── .gitkeep
        └── index.ts          # Main feature export
```

### Monorepo Setup

In a monorepo with `apps/web`:

```text
apps/web/components/
├── routes/
│   └── dashboard/
│       ├── create/
│       │   ├── index.ts
│       │   └── .gitkeep
│       ├── read/
│       │   ├── DashboardView.tsx
│       │   ├── index.ts
│       │   └── .gitkeep
│       └── index.ts
└── shared/
    ├── header/
    │   ├── create/
    │   │   ├── index.ts
    │   │   └── .gitkeep
    ├── hooks/
    │   ├── useHeader.ts
    │   ├── index.ts
    │   └── .gitkeep
    └── index.ts
```

## Building and Installation

### Build from Source

```bash
cd validate-frontend-structure
go build -o validate-frontend-structure
```

Requires Go 1.25.5 or later.

### Install as Claude Hook

```bash
cp validate-frontend-structure ~/.claude/hooks/
```

The hook will automatically be loaded when enabled via `CLAUDE_HOOKS_AST_VALIDATION=true`.

## Testing

Run the comprehensive test suite:

```bash
cd validate-frontend-structure
go test -v
```

Tests cover:
- Project root discovery
- Feature structure validation
- Loose component detection
- Multiple error scenarios
- Operation type filtering
- Configuration loading

## Architecture Details

### How It Works

1. **Hook invocation**: Claude Code calls the hook before executing structure-modifying operations
2. **JSON parsing**: Hook reads JSON from stdin containing tool name and file path
3. **Operation filtering**: Determines if the operation modifies component structure
4. **Project discovery**: Searches upward for `package.json` to find project root
5. **Structure validation**: Checks all features in `components/routes/` and `components/shared/`
6. **Error reporting**: Lists all violations found, helps developer understand what to fix
7. **Exit code**: Returns 0 (allow) or 2 (block) based on validation result

### Graceful Degradation

The hook fails safely and allows operations if:
- JSON input cannot be parsed
- Project root cannot be found
- Components directory doesn't exist
- The hook is not enabled

This prevents the tool from breaking workflows in unusual project structures.

## Related Skills and Tools

- **frontend-architecture**: Claude skill that provides patterns for organizing React/Next.js projects
- **code-analysis**: Lint and typecheck analysis for monorepos
- **.claude-hooks-config.sh**: Project configuration file for enabling hooks

For detailed architecture guidance, see: `~/.claude/skills/frontend-architecture/SKILL.md`

## Troubleshooting

### Hook not running?

Check that `CLAUDE_HOOKS_AST_VALIDATION=true` is set:
```bash
echo $CLAUDE_HOOKS_AST_VALIDATION  # Should print 'true'
```

### Different structure than expected?

This tool enforces a specific CRUD-based pattern. If your project uses a different architecture, you may want to disable the hook with `CLAUDE_HOOKS_AST_VALIDATION=false` or not set it.

### Can't find project root?

Ensure your project has a `package.json` file in the root. The hook uses this to identify the project boundary.

### Getting false positives?

Report issues with specific project structures. The tool may need adjustment for edge cases in monorepos or non-standard layouts.
