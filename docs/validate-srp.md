# validate-srp Documentation

**Repository:** [https://github.com/milehighideas/validate-srp](https://github.com/milehighideas/validate-srp)


## Overview

`validate-srp` is a Single Responsibility Principle (SRP) validator for TypeScript/TSX files. It enforces architectural patterns in frontend applications, particularly for React/Next.js projects using Convex as a backend. The tool integrates with Claude hooks to validate code during development and can also run as a standalone CLI.

## Purpose

The validator ensures that TypeScript/TSX components follow SRP by checking:

- Direct imports from Convex libraries (must use data-layer abstraction)
- State management placement (useState must be in content components, not screens)
- Single export per file (enforced for CRUD operations)
- File size limits (screens: 100 lines, hooks: 150 lines, components: 200 lines)
- Type export locations (must be in types/ folders)
- Mixed concerns detection (data fetching + UI + state in single file)

## Usage Modes

### Standalone CLI Mode

Run validation on single files or entire directories:

```bash
# Check a single file
validate-srp --file path/to/Component.tsx

# Check all TypeScript files in a directory
validate-srp --path ./src/components

# Show verbose output (including passed files)
validate-srp --path ./src -v
validate-srp --file MyComponent.tsx --verbose

# Display help
validate-srp -h
validate-srp --help
```

### Claude Hook Mode

The tool automatically integrates with Claude's development environment when invoked from Claude Code. In hook mode, it:

- Reads JSON input from stdin (tool invocations and file operations)
- Validates TypeScript operations in real-time
- Only activates when opt-in environment variable is set
- Reports violations as blocking errors or warnings

Example hook invocation (automatic):
```bash
echo '{"tool_name":"Write","tool_input":{"file_path":"Component.tsx","content":"..."}}' | validate-srp
```

## Command Line Arguments

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--path <dir>` | - | Recursively check all TypeScript files in directory |
| `--file <file>` | - | Check a single TypeScript file |
| `--verbose` | `-v` | Show verbose output, including files that pass validation |
| `--help` | `-h` | Display help message and exit |

### Usage Examples

```bash
# Validate entire src directory with verbose output
validate-srp --path ./src --verbose

# Check a specific component file
validate-srp --file ./src/components/UserProfile.tsx

# Validate components directory quietly (only show violations)
validate-srp --path ./src/components
```

## Environment Variables

### `CLAUDE_HOOKS_AST_VALIDATION`

Controls whether validation runs in hook mode. This is **opt-in only**.

- **Value**: `"true"` or `"false"`
- **Default**: If not set, hook mode is disabled
- **Effect**: When set to `"true"`, validation runs on TypeScript file operations
- **Source**: Can be set in `.claude-hooks-config.sh` or via shell environment

Example configuration:
```bash
export CLAUDE_HOOKS_AST_VALIDATION=true
```

### Project Configuration

Projects can configure validation via `.claude-hooks-config.sh` in the root directory:

```bash
#!/bin/bash
export CLAUDE_HOOKS_AST_VALIDATION=true
```

## Validation Checks

### 1. Direct Convex Imports (Error)

**Rule**: Components cannot import directly from Convex libraries outside the data-layer.

**Violations**:
- `import { useQuery } from 'convex/react'`
- `import { api } from '../_generated/api'`

**Exceptions**:
- Files in `/data-layer/`, `/backend/`, `/convex/`, `/scripts/`, `/providers/` folders
- `_layout.tsx` files (infrastructure components)
- Allowed imports: `Preloaded`, `usePreloadedQuery`

**Fix**: Use data-layer hooks instead: `import { useUser } from '@dashtag/data-layer/generated-hooks'`

### 2. State in Screens (Error)

**Rule**: Screen files cannot use state management hooks (useState, useReducer, useContext).

**Violation**:
```typescript
// File: screens/HomeScreen.tsx
const [count, setCount] = useState(0); // ❌ Error
```

**Fix**: Move state to content components or custom hooks.

### 3. Multiple Exports in CRUD Files (Error)

**Rule**: Files in `/create/`, `/read/`, `/update/`, or `/delete/` folders must export only one component.

**Violation**:
```typescript
// File: features/users/create/CreateUserForm.tsx
export const CreateUserForm = () => <div />;
export const helper = () => {}; // ❌ Error: multiple exports
```

**Fix**: Move helper to separate file or inline it.

### 4. File Size Limits (Warning)

**Rule**: Large files indicate potential SRP violations.

**Limits**:
- Screens: 100 lines maximum
- Hooks: 150 lines maximum
- Components: 200 lines maximum
- Scripts: Unlimited

**Example Warning**:
```text
Screen file is 150 lines (limit: 100)
→ Screens should only handle navigation - move logic to content component
```

### 5. Type Export Location (Error)

**Rule**: Type definitions must be in `/types/` folders, not component files.

**Violation**:
```typescript
// File: components/UserProfile.tsx
export type UserProps = { id: string }; // ❌ Error
```

**Fix**: Move to types folder:
```typescript
// File: types/User.ts
export type UserProps = { id: string };

// Then in component:
import type { UserProps } from '../types/User';
```

### 6. Mixed Concerns (Warning)

**Rule**: Files shouldn't mix data fetching + UI + state management.

**Violation**:
```typescript
// File: components/UserCard.tsx
import { useUser } from '@dashtag/data-layer/generated-hooks'; // Data
import { Button } from '@/components/ui/button'; // UI
import { useState } from 'react'; // State

const [isEditing, setIsEditing] = useState(false); // ❌ Warning: 3 concerns
```

**Fix**: Separate into appropriate layers:
- Data fetching → custom hooks
- State management → custom hooks or components
- UI rendering → functional components

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success: No violations found |
| `1` | Error: Tool execution failed (file not found, permission denied, etc.) |
| `2` | Validation failed: SRP violations detected |

## Output Format

### Standalone Mode (CLI)

```typescript
Checking 5 TypeScript file(s) for SRP compliance...

✅ src/components/Button.tsx

============================================================
  SRP CHECK RESULTS
============================================================

⚠️  WARNINGS (1):

  src/hooks/useUser.tsx:
    Hook file is 175 lines (limit: 150)
    → Consider splitting into smaller, focused hooks

❌ ERRORS (2):

  src/components/UserForm.tsx:
    🚨 ARCHITECTURAL VIOLATION: Direct Convex imports forbidden outside data-layer
    FIX: Use data-layer hooks instead (import from @dashtag/data-layer/generated-hooks)

  src/types.tsx:
    Type exports found outside types/ folder: UserType
    FIX: Move type definitions to ../types/ folder for better organization and reusability

Files checked: 5
Errors: 2, Warnings: 1

❌ SRP check failed
```

### Hook Mode

When validation fails in hook mode:

```bash
❌ BLOCKED: SRP violation in Component.tsx
============================================================

  ✗ 🚨 ARCHITECTURAL VIOLATION: Direct Convex imports forbidden outside data-layer
    FIX: Use data-layer hooks instead (import from @dashtag/data-layer/generated-hooks)

============================================================
HOW TO FIX:
  1. Move direct Convex imports to data-layer hooks
  2. Move state (useState) from screens to content components
  3. Split files with multiple exports into separate files
  4. Move 'export type' definitions to types/ folder

See: ~/.claude/skills/frontend-architecture/SKILL.md
```

## Example Usage

### Check single component:

```bash
$ validate-srp --file ./src/components/UserProfile.tsx

Checking 1 TypeScript file(s) for SRP compliance...

============================================================
  SRP CHECK RESULTS
============================================================

Files checked: 1
Errors: 0, Warnings: 0

✅ SRP check passed
```

### Check entire directory with violations:

```bash
$ validate-srp --path ./src/features

Checking 12 TypeScript file(s) for SRP compliance...

✅ src/features/users/create/CreateUserForm.tsx
✅ src/features/users/read/UserList.tsx
...

============================================================
  SRP CHECK RESULTS
============================================================

❌ ERRORS (1):

  src/features/users/components/UserCard.tsx:
    Type exports found outside types/ folder: CardProps, CardState
    FIX: Move type definitions to ../types/ folder for better organization and reusability

Files checked: 12
Errors: 1, Warnings: 0

❌ SRP check failed
```

### Verbose output with passing files:

```bash
$ validate-srp --path ./src/components -v

Checking 3 TypeScript file(s) for SRP compliance...

✅ src/components/Button.tsx
✅ src/components/Card.tsx
✅ src/components/Header.tsx

============================================================
  SRP CHECK RESULTS
============================================================

Files checked: 3
Errors: 0, Warnings: 0

✅ SRP check passed
```

## Integration with Claude Hooks

The tool is designed to work seamlessly with Claude Code's hook system:

1. **Automatic invocation**: Runs on TypeScript file writes via the Write, Edit, and Bash tools
2. **Opt-in only**: Requires `CLAUDE_HOOKS_AST_VALIDATION=true` in project config
3. **Blocking errors**: SRP violations exit with code 2, preventing code generation
4. **Non-blocking warnings**: Displayed but don't stop execution
5. **Configuration**: Load from `.claude-hooks-config.sh` in project root

## File Detection

The tool processes:
- `.tsx` files (React TypeScript components)
- `.ts` files (TypeScript utilities)

**Excludes**:
- `.d.ts` type definition files
- Test files (`*.test.tsx`, `*.spec.tsx`)
- Hidden directories (starting with `.`)
- `node_modules/`, `dist/`, `build/` directories

## Architecture Compliance Reference

For detailed information about the architectural patterns enforced by this tool, see the frontend architecture skill documentation at `~/.claude/skills/frontend-architecture/SKILL.md`.

Key principles:
- **Data-layer abstraction**: Centralize Convex integration
- **Screen-content separation**: Screens handle routing, content handles logic
- **Single responsibility**: Each file has one clear purpose
- **Type organization**: Keep types separate and discoverable
