# block-lint-workarounds

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/block-lint-workarounds`)

## Description

A Claude hook that enforces code quality by detecting and preventing common linting workarounds. It intercepts file write/edit operations via the Edit and Write tools to identify problematic patterns:

- **Underscore prefix workarounds** - Using underscore-prefixed aliases or destructuring to silence unused variable warnings
- **ESLint suppression comments** - Direct eslint-disable directives that hide linting errors
- **TypeScript suppression comments** - TypeScript type-checking suppression directives (detected with warnings only)

The tool enforces the philosophy that code should be fixed rather than linting errors suppressed.

## Usage

This tool is designed to be used as a **Claude hook** that runs automatically when using the Edit and Write tools. It reads JSON-formatted hook input from stdin and outputs a decision (approve/block) along with an optional reason.

### As a Claude Hook

The tool is configured to run as part of the Claude Code hook system. When Edit or Write tools are invoked, the hook:

1. Receives JSON input describing the tool operation
2. Checks the file content for problematic patterns
3. Returns a decision to approve or block the operation

### Command Line Usage

```bash
./block-lint-workarounds < input.json > output.json
```

## Input Format

The tool expects JSON input on stdin with the following structure:

```json
{
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "/path/to/file.ts",
    "new_string": "const x = 1;"
  }
}
```

### Input Fields

- **tool_name** (string, required): Name of the tool being executed (only "Edit" and "Write" are checked)
- **tool_input** (object, required): Tool-specific parameters
  - **file_path** (string): Path to the file being modified
  - **new_string** (string): New content being written (for Edit tool)
  - **content** (string): Content being written (for Write tool)

## Output Format

The tool produces JSON output to stdout:

```json
{
  "decision": "approve",
  "reason": "Optional explanation"
}
```

### Output Fields

- **decision** (string): Either "approve" or "block"
- **reason** (string, optional): Explanation of the decision (included when blocking or warning)

## Detection Rules

### 1. Underscore Prefix Workarounds (BLOCKED)

Detects three patterns that use underscore prefixes to suppress unused variable warnings:

- **Import aliases**: Using underscore prefix in import alias syntax
- **Destructuring**: Using underscore prefix in object destructuring
- **Type aliases**: Using underscore prefix in type alias declarations

**Exception**: Convex system fields (\_id, \_creationTime) are allowed in files within `/convex/` directories, as these are valid Convex schema fields.

### 2. ESLint Disable Comments (BLOCKED)

Detects ESLint suppression comments:

- **Inline**: ESLint disable directives in single-line comments
- **Block**: ESLint disable directives in block comments

These are always blocked regardless of context.

### 3. TypeScript Suppression Comments (WARNED, NOT BLOCKED)

Detects TypeScript type checking suppression directives:

- @ts-ignore
- @ts-expect-error
- @ts-nocheck

These produce a warning but still approve the operation, as TypeScript suppression is sometimes necessary for:

- Deep type instantiation limits (TS2589)
- Third-party library type issues
- Complex generic inference constraints

## Environment Variables

None. The tool does not use environment variables.

## Exit Codes

- **0**: Successful execution (regardless of approve/block decision)
- **1**: Error during execution (invalid JSON, I/O error, etc.)

## Examples

### Example 1: Blocking Underscore Prefix Pattern

When attempting to write code with underscore-prefixed import alias or destructuring, the hook blocks the operation:

```bash
Input: Code with underscore prefix pattern
Output: BLOCKED decision with explanation to remove unused imports/variables
         or use 'import type' syntax for type-only imports
```

### Example 2: Blocking ESLint Suppression

When attempting to add eslint-disable comments:

```text
Input: Code with eslint-disable comment
Output: BLOCKED decision with explanation to fix underlying issues instead
```

### Example 3: Warning on TypeScript Suppression

When attempting to add TypeScript suppression directives:

```text
Input: Code with @ts-ignore or similar directive
Output: APPROVE decision with WARNING reason explaining when
        TypeScript suppression may be necessary
```

### Example 4: Allowing Convex System Fields

When editing files in the /convex/ directory:

```text
Input: Convex schema file with _id or _creationTime fields
Output: APPROVE decision (no blocking or warning)
```

### Example 5: Ignoring Non-Edit/Write Tools

The hook only checks Edit and Write tools:

```text
Input: Read tool or other tool with problematic patterns
Output: APPROVE decision (hook doesn't examine non-Edit/Write tools)
```

## Configuration

The tool has no configuration options. Detection patterns are hardcoded and cannot be customized.

### Detection Patterns (Regex)

The tool uses the following regex patterns for detection:

| Pattern                 | Description                         |
| ----------------------- | ----------------------------------- |
| `as\s+_\w+`             | Import/type aliases with underscore |
| `:\s*_\w+`              | Destructuring with underscore       |
| `type\s+\w+\s+as\s+_`   | Type alias with underscore          |
| `//\s*eslint-disable`   | Inline ESLint suppression           |
| `/\*\s*eslint-disable`  | Block ESLint suppression            |
| `//\s*@ts-ignore`       | TypeScript ignore directive         |
| `//\s*@ts-expect-error` | TypeScript expect-error directive   |
| `//\s*@ts-nocheck`      | TypeScript nocheck directive        |

## Behavior Notes

- The hook only examines the Edit and Write tools. All other tools pass through automatically.
- Empty content is automatically approved (no patterns to match).
- The tool reads from stdin and writes to stdout, making it suitable for piping and integration with other tools.
- File path context is used only for detecting Convex directories (for system field exceptions).
- Detection is case-sensitive and pattern-based using Go regular expressions.

## Building

To build the binary from source:

```bash
cd /Volumes/Developer/code/shared/claude-hooks/block-lint-workarounds
go build -o block-lint-workarounds main.go
```

## Testing

Run the comprehensive test suite:

```bash
cd /Volumes/Developer/code/shared/claude-hooks/block-lint-workarounds
go test -v
```

The test suite includes:

- Individual pattern detection tests
- Convex file exception tests
- Integration tests for Edit and Write tools
- End-to-end tests with complete JSON flows
- Edge case tests (empty content, missing fields, etc.)

## Source Files

- **main.go**: Core implementation with pattern detection logic and hook interface
- **main_test.go**: Comprehensive test suite with 30+ test cases covering all detection patterns and edge cases
