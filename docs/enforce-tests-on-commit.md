# Enforce Tests on Commit Hook

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/enforce-tests-on-commit`)

## Overview

`enforce-tests-on-commit` is a Claude hook that enforces test coverage requirements when committing code changes. It intercepts `git commit` commands and ensures that all modified source files have corresponding test files and that those tests pass before allowing the commit to proceed.

This hook is designed for monorepo environments with multiple project types (backend, mobile, web, portal) and implements intelligent test requirement detection, automatically skipping tests for certain file types like type definitions, mock files, and re-export modules.

## Purpose

The hook prevents incomplete or undertested code from being committed by:

1. **Blocking commits** when source files lack corresponding test files
2. **Running tests** for modified files and ensuring they pass
3. **Validating test organization** (co-located with source files, not in `__tests__/` folders)
4. **Smart skipping** of test requirements for non-testable files
5. **Session-aware tracking** of which files have been edited by Claude

## Usage

### How to Use

The tool is used as a Claude pre-commit hook. It reads JSON input from stdin containing:

- The git command being executed
- The current working directory
- The Claude session ID (for tracking edited files)

When a `git commit` command is detected, the hook:

1. Checks for test files in `__tests__/` folders (blocks these)
2. Loads tracked files from the session
3. Intersects staged files with session-tracked files
4. Verifies corresponding test files exist
5. Runs tests to ensure they pass
6. Either allows or blocks the commit

### Command Line Arguments

This tool is typically invoked automatically by the Claude editor and does not accept command-line arguments directly.

However, it can be tested by providing JSON input to stdin:

```bash
echo '{"tool_input":{"command":"git commit -m \"test\""},"session_id":"test-session","cwd":"/path/to/repo"}' | ./enforce-tests-on-commit
```

### Environment Variables

- **HOME** - Used to locate the `.claude/sessions/` directory where session tracking data is stored

## Exit Codes

- **0 (exitAllow)** - Commit is allowed to proceed
- **2 (exitBlock)** - Commit is blocked due to test requirement failures

## Input Format

The tool expects JSON input on stdin with the following structure:

```json
{
  "tool_input": {
    "command": "git commit -m 'message'"
  },
  "session_id": "session-uuid",
  "cwd": "/path/to/working/directory"
}
```

## Supported Project Types

The tool recognizes and handles four project types in a monorepo structure:

- **backend** - Located at `packages/backend/` (uses Vitest with `npm run test:run`)
- **mobile** - Located at `apps/mobile/` (uses Jest with `npm run test`)
- **web** - Located at `apps/web/` (uses Vitest with `npm run test:run`)
- **portal** - Located at `apps/portal/` (uses Vitest with `npm run test:run`)

## Core Features

### Test File Co-Location

Tests must be co-located with their source files:

- Source: `src/components/Button.tsx` → Test: `src/components/Button.test.tsx`
- Source: `src/utils.ts` → Test: `src/utils.test.ts`

The tool blocks commits that add test files to `__tests__/` folders, suggesting they be moved next to their source files instead.

### Smart Test Skipping

The following files are automatically excluded from test requirements:

- **Type-only changes** - Files with only TypeScript type/interface changes
- **Mock files** - Files in `/__mocks__/` directories
- **Testing utilities** - Files in `/testing/` directories
- **Type definitions** - Files in `/types/` directories or with `.types.ts/.types.tsx` extensions
- **Layout files** - Root layout files (`_layout.tsx`, `_layout.ts`)
- **Configuration files** - `jest.config.*`, `vitest.config.*`, `tsconfig.*`, `babel.config.*`, etc.
- **Type declaration files** - `.d.ts` files
- **Setup files** - `jest.setup.*`, `vitest.setup.*`
- **Auth UI components** - Components matching patterns like `social-connections`, `sign-in-form`, `oauth-callback`
- **Re-export modules** - Files that only contain export statements (detected automatically)
- **Validator/constant files** - Files in `/lib/validators/` or `/lib/constants/`

### Session-Aware Tracking

The hook uses Claude session tracking to determine which files were edited:

1. Loads `~/.claude/sessions/{session_id}.json` containing tracked source and test files
2. Cleans stale entries (removes files that no longer exist)
3. Intersects staged files with session-tracked files
4. Only enforces test requirements on files Claude touched that are being committed

This prevents the hook from blocking commits of unrelated files or pre-existing code.

### Type-Only Change Detection

The hook intelligently detects when changes are purely type-related and skips test requirements:

- Strips type assertions (`as const`, `as Type`, etc.)
- Recognizes type-only imports and exports
- Matches changed lines after stripping type information
- Detects type import additions without other changes

### Test Timeout

Tests have a 120-second timeout. If tests take longer, the commit is blocked with a timeout error.

### Vitest Setup Validation

For web and portal projects, the hook validates that Vitest is properly configured:

- Checks that `package.json` contains a `test:run` script
- Verifies `vitest.config.ts` exists
- Blocks commits with helpful instructions if setup is incomplete

## Example Usage Scenarios

### Scenario 1: Commit with Complete Test Coverage

```bash
# User has edited Button.tsx and Button.test.tsx
git add Button.tsx Button.test.tsx
git commit -m "feat: add Button component"
# ✅ Commit proceeds: Tests run and pass
```

### Scenario 2: Commit Missing Test File

```bash
# User has edited Utils.ts but forgot to create Utils.test.ts
git add Utils.ts
git commit -m "feat: add utility function"
# ❌ Commit blocked: Missing test file at src/Utils.test.ts
```

### Scenario 3: Commit with Test File in **tests**/

```bash
# User created Button.test.tsx in __tests__/ instead of co-locating
git add Button.tsx __tests__/Button.test.tsx
git commit -m "feat: add Button"
# ❌ Commit blocked: Test files should not be in __tests__/ folders
# Suggestion: Move to src/components/Button.test.tsx
```

### Scenario 4: Type-Only Changes

```bash
# User added a TypeScript type import but no logic change
git add types.ts  # Only added: import type { Foo } from './foo'
git commit -m "chore: add Foo type import"
# ✅ Commit proceeds: Type-only changes don't require tests
```

### Scenario 5: Config or Setup File

```bash
# User updated vitest.config.ts
git add vitest.config.ts
git commit -m "chore: update Vitest config"
# ✅ Commit proceeds: Config files don't require tests
```

## Key Functions

### Test Requirement Functions

- **`shouldSkipTestRequirement(filePath)`** - Determines if a file should be excluded from test requirements
- **`getTestPathForSource(sourcePath)`** - Maps source file to expected co-located test file path
- **`isTypeOnlyChange(filePath)`** - Analyzes git diff to detect purely type-related changes

### Project Detection

- **`getProjectType(filePath)`** - Identifies which project (backend, mobile, web, portal) a file belongs to
- **`findProjectRoot(filePath)`** - Locates the project root directory by finding `package.json` and project markers

### Test Execution

- **`runTests(testFiles, projectType, projectRoot)`** - Executes tests using appropriate test runner for the project type
- **`checkVitestSetup(projectRoot)`** - Validates Vitest configuration for web/portal projects

### Session Management

- **`loadSessionData(sessionID)`** - Loads tracked files from `~/.claude/sessions/{sessionID}.json`
- **`saveSessionData(sessionID, sessionData)`** - Persists session tracking data
- **`cleanStaleEntries(sessionData)`** - Removes entries for deleted files

### Git Operations

- **`getGitStagedFiles(cwd)`** - Gets absolute paths of files staged for commit
- **`isGitCommit(command)`** - Detects if command is a git commit (but not `--amend`)

## Blocking Conditions

The commit is blocked (exit code 2) when:

1. Test files are added to `__tests__/` folders instead of being co-located
2. A source file lacks a corresponding test file
3. Tests exist but fail to pass
4. Tests timeout after 120 seconds
5. Vitest is not properly configured for web/portal projects

## Allowed Amends

The hook allows `git commit --amend` to proceed without re-running tests. This is used for fixing pre-commit hook issues without re-testing all code.

## Output

When blocking a commit, the hook writes error messages to stderr with:

- Clear explanation of why the commit was blocked
- List of affected files
- Suggestions for fixes (e.g., where to move test files)
- Test failure output (last 3000 characters)

When allowing a commit with tests, it writes to stderr:

- `✅ N test file(s) passed`

## Implementation Details

### Hybrid Approach

The hook uses a hybrid approach combining git and session tracking:

1. **Git as source of truth** - Git's staged files are always accurate
2. **Session as scope filter** - Only enforces on files Claude touched
3. **Intersection** - Only blocks if file is BOTH staged AND in session
4. **Self-healing** - Automatically removes deleted files from session tracking

This prevents false positives while ensuring files edited by Claude are properly tested.

### Project Root Discovery

The hook walks up the directory tree to find the project root by looking for:

1. `package.json` + `convex/` → Backend project
2. `package.json` + `app.json` → Mobile app
3. `package.json` + path containing `apps/web` → Web project
4. `package.json` + path containing `apps/portal` → Portal project
5. `package.json` with `packages/` directory → Monorepo root (keep walking up)

### Re-export Module Detection

Files are detected as re-export modules if:

- They have ≤10 lines of code
- All non-comment lines start with `export` or `'use client'`/`"use server"` directives
- This prevents unnecessary test files for barrel exports

## Testing the Hook

The repository includes comprehensive tests in `main_test.go` covering:

- Git commit detection
- Test path resolution
- Project type identification
- Test skipping logic
- Re-export module detection
- Session data persistence
- File intersection operations
- Type-only change detection

Run tests with:

```bash
go test ./enforce-tests-on-commit
```
