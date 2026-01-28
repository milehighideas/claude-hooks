# validate-test-files

**Repository:** [https://github.com/milehighideas/validate-test-files](https://github.com/milehighideas/validate-test-files)


## Overview

`validate-test-files` is a Claude hook that validates test file requirements for component files in multi-app TypeScript/React codebases. It enforces testing standards by blocking Write and Edit operations on components that don't have the required test files, helping maintain code quality and test coverage across mobile, native, web, and portal applications.

## Purpose

The tool ensures that:
- Screen components have both unit tests and E2E tests
- Form components (create/update CRUD) have both unit tests and E2E tests
- Hooks and utilities have unit tests
- Interactive components have both unit tests and E2E tests
- Display-only components have unit tests
- Test requirements match the appropriate app type (mobile/native use Maestro, web/portal use TypeScript)

## Usage

### As a Claude Hook

This tool is designed to run as a pre-commit hook in the Claude hooks system. It intercepts Write and Edit operations on component files and validates test requirements before allowing the operation to proceed.

The hook receives JSON input via stdin containing:
- `tool_name`: The name of the tool being invoked ("Write" or "Edit")
- `tool_input.file_path`: The absolute path to the file being modified

### Command Line Usage

```bash
# Run directly with JSON input via stdin
echo '{"tool_name":"Write","tool_input":{"file_path":"/path/to/component.tsx"}}' | ./validate-test-files
```

## How It Works

### File Classification

The tool classifies files to determine test requirements:

1. **Screens**: Files in `/screens/` directories
   - Requires: Unit test (`.test.tsx`) + E2E test
   - E2E format depends on app type

2. **CRUD Components**: Files in `/create/`, `/update/`, or `/delete/` directories
   - Create/Update (forms): Require unit test + E2E test
   - Delete: Requires unit test only
   - Reason: Forms are interactive user input handlers

3. **Hooks and Utilities**: Files in `/hooks/` or `/utils/` directories
   - Requires: Unit test only
   - Reason: Business logic without user interaction

4. **Interactive Components**: Components that use state or form management hooks
   - Detected patterns:
     - State hooks: `useState`, `useReducer`, `useContext`
     - Query/mutation hooks: `useMutation`, `useQuery`
     - Form hooks: `useForm`, `useFormState`, `useFormContext`, `useController`
   - Requires: Unit test + E2E test

5. **Display-Only Components**: Components with no interactive patterns
   - Requires: Unit test only

### File Classification Priority

Files are skipped from validation if they are:
- Already test files (containing `.test.`, `.spec.`, `.e2e.`, or `.maestro.`)
- Type definition files (`/types/` directories)
- Barrel exports (`index.ts` or `index.tsx`)

### Test File Path Generation

#### Unit Tests
- `.tsx` files → `.test.tsx`
- `.ts` files → `.test.ts`
- Example: `components/Button.tsx` → `components/Button.test.tsx`

#### E2E Tests
Mobile/Native apps:
- Use Maestro YAML format: `.maestro.yaml`
- Example: `screens/Home.tsx` → `screens/Home.maestro.yaml`

Web/Portal apps:
- Use TypeScript format: `.e2e.ts`
- Example: `components/UserForm.tsx` → `components/UserForm.e2e.ts`

## Command Line Arguments

The tool does not accept command line arguments. All behavior is controlled via:
- JSON input via stdin
- Environment variables
- File path and content analysis

## Environment Variables

### CLAUDE_HOOKS_AST_VALIDATION

Controls whether test file validation is enabled.

- `CLAUDE_HOOKS_AST_VALIDATION=false`: Disables the hook (allows operations without test files)
- Any other value or unset: Hook is enabled (default behavior)

**Example:**
```bash
export CLAUDE_HOOKS_AST_VALIDATION=false
echo '{"tool_name":"Write","tool_input":{"file_path":"/path/to/component.tsx"}}' | ./validate-test-files
# Operation succeeds even without test files
```

## Exit Codes

- **0**: Operation allowed (no violations found, or validation is disabled)
- **2**: Operation blocked (test file requirements not met)

### Exit Code Behavior

The tool allows operations in these cases (exits 0):
- Validation is disabled via environment variable
- File is a test file (already tested)
- File is a type definition or barrel export
- File is not a `.ts` or `.tsx` file
- App type cannot be determined
- File content cannot be read (graceful failure)
- No test violations found

The tool blocks operations (exits 2) only when:
- Required test files are missing AND
- The validation is enabled

## Output

### Success

When validation passes or the hook is disabled, no output is produced.

### Failure

When test requirements are not met, an error message is written to stderr with:

```sql
BLOCKED: Test file requirements not met

File: Component.tsx

Missing tests:

  ❌ Missing unit test: Component.test.tsx
     Reason: Screen components require tests
     Expected: /full/path/to/Component.test.tsx

  ❌ Missing E2E test: Component.maestro.yaml
     Reason: Screen components require tests
     Expected: /full/path/to/Component.maestro.yaml
     App type: mobile

Test requirements:
  - Screens: Unit test (.test.tsx) + E2E test
  - Forms (create/update): Unit test + E2E test
  - Hooks/Utils: Unit test only
  - Interactive components: Unit test + E2E test
  - Display components: Unit test only

E2E test types:
  - mobile/native: .maestro.yaml
  - web/portal: .e2e.ts

To fix:
1. Create the missing test files
2. Or set CLAUDE_HOOKS_AST_VALIDATION=false to disable
```

## Example Usage

### Example 1: Blocking Screen Component Without Tests

```bash
# Command
echo '{
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/project/packages/mobile/src/screens/Home.tsx"
  }
}' | ./validate-test-files

# Exit code: 2
# Output to stderr: Detailed message about missing tests
```

### Example 2: Allowing Component With Proper Tests

```bash
# Files exist:
# - /project/packages/web/src/components/Button.tsx
# - /project/packages/web/src/components/Button.test.tsx
# - /project/packages/web/src/components/Button.e2e.ts

echo '{
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/project/packages/web/src/components/Button.tsx"
  }
}' | ./validate-test-files

# Exit code: 0
# No output
```

### Example 3: Bypassing Validation

```bash
export CLAUDE_HOOKS_AST_VALIDATION=false

echo '{
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/project/packages/mobile/src/screens/Home.tsx"
  }
}' | ./validate-test-files

# Exit code: 0
# No output - validation is disabled
```

### Example 4: Display Component Only Needs Unit Test

```bash
# Component without interactive patterns
# - /project/packages/portal/src/components/Header.tsx
# - /project/packages/portal/src/components/Header.test.tsx exists
# - No E2E test needed

echo '{
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/project/packages/portal/src/components/Header.tsx"
  }
}' | ./validate-test-files

# Exit code: 0
# No output - only unit test is required for display components
```

## Test Classification Rules

### App Type Detection

The tool determines app type from the file path:
- Contains `/mobile/` → mobile app (E2E format: `.maestro.yaml`)
- Contains `/native/` → native app (E2E format: `.maestro.yaml`)
- Contains `/web/` → web app (E2E format: `.e2e.ts`)
- Contains `/portal/` → portal app (E2E format: `.e2e.ts`)

### Interactive Component Detection

Components are considered interactive if they contain:

**State management hooks:**
- `useState(...)`
- `useReducer(...)`
- `useContext(...)`

**Data mutation/query hooks:**
- `useMutation(...)`
- `useQuery(...)`

**Form management hooks:**
- `useForm`
- `useFormState`
- `useFormContext`
- `useController`

These are detected via regex pattern matching against the component source code.

## Validation Scope

The tool validates:
- Only `.ts` and `.tsx` files in `/components/` directories
- Only on Write and Edit tool operations
- Files must be actual components (filtered by location)

The tool does NOT validate:
- Other file types (`.js`, `.jsx`, `.css`, etc.)
- Files outside `/components/` directories
- Read, Delete, or other non-Write/Edit operations
- Files that are already test files
- Type definition files
- Barrel exports (index files)

## File Path Examples

### Mobile Screen (requires unit + E2E Maestro)
```text
/project/packages/mobile/src/screens/Login.tsx
→ /project/packages/mobile/src/screens/Login.test.tsx
→ /project/packages/mobile/src/screens/Login.maestro.yaml
```

### Web Form Component (requires unit + E2E TypeScript)
```text
/project/packages/web/src/components/create/UserForm.tsx
→ /project/packages/web/src/components/create/UserForm.test.tsx
→ /project/packages/web/src/components/create/UserForm.e2e.ts
```

### Hook (requires unit test only)
```text
/project/packages/portal/src/hooks/useAuth.ts
→ /project/packages/portal/src/hooks/useAuth.test.ts
```

### Display Component (requires unit test only)
```text
/project/packages/portal/src/components/Header.tsx
→ /project/packages/portal/src/components/Header.test.tsx
```

## Error Handling

The tool gracefully handles errors:
- If JSON input cannot be parsed, the operation is allowed (exits 0)
- If a file cannot be read during interactive detection, validation is skipped (exits 0)
- File system errors do not block operations
- Missing app type just skips E2E validation

This permissive approach ensures the hook never breaks the development workflow unexpectedly.

## Integration

The tool is designed to be used as part of the Claude hooks system:

1. The hook receives tool invocations as JSON via stdin
2. It parses the tool name and file path
3. It validates test requirements for component files
4. It exits with code 0 to allow or 2 to block the operation
5. Error messages are written to stderr for visibility

See the claude-hooks documentation for integration details.
