# Pre-commit Hook Tool

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/pre-commit`)

A comprehensive Go-based pre-commit hook that validates code quality, enforces architectural patterns, and prevents common issues before code is committed. Runs checks in parallel for speed and provides detailed reporting.

## Overview

The pre-commit tool is designed to be used as a Git hook that runs automatically before commits, but can also run standalone for checking arbitrary directories. It performs multiple validation checks simultaneously to catch issues early in the development process.

## Usage

### As a Git Pre-commit Hook (Default)

The tool automatically detects staged files and runs applicable checks:

```bash
pre-commit
```

The tool will:

1. Load configuration from `.pre-commit.json`
2. Identify affected apps and shared packages
3. Run enabled checks in parallel
4. Report failures with actionable errors

### Standalone Mode

Check all files in a directory without Git context:

```bash
pre-commit --standalone --path <directory>
```

Example:

```bash
pre-commit --standalone --path ./apps/mobile
```

### Run Specific Check

Run only one check instead of all enabled checks:

```bash
pre-commit --check <check-name>
```

## Command Line Arguments and Flags

### Primary Flags

- `--standalone` - Run without git context, checking all files in a path
- `--path <directory>` - Directory to check in standalone mode (required with `--standalone`)
- `--check <check-name>` - Run only a specific check by name
- `--list` - List all available checks
- `--config <path>` - Path to `.pre-commit.json` config file (defaults to project root)
- `--report-dir <directory>` - Write detailed analysis reports to this directory

### Examples

```bash
# List all available checks
pre-commit --list

# Run only SRP check
pre-commit --check srp

# Check specific directory standalone
pre-commit --standalone --path ./packages/ui

# Use custom config file
pre-commit --config ./custom-config.json

# Generate detailed reports
pre-commit --report-dir ./analysis-reports
```

## Available Checks

Run `pre-commit --list` to see all available checks. Currently supported:

| Check               | Purpose                                               |
| ------------------- | ----------------------------------------------------- |
| `lintTypecheck`     | ESLint and TypeScript type checking                   |
| `tests`             | Run test suites for affected apps                     |
| `changelog`         | Validate changelog entries exist                      |
| `consoleCheck`      | Check for console.log statements                      |
| `frontendStructure` | Validate CRUD folder structure in components          |
| `srp`               | Single Responsibility Principle validation            |
| `mockCheck`         | Ensure tests use `__mocks__/` instead of inline mocks |
| `testFiles`         | Ensure test files exist for source files              |
| `vitestAssertions`  | Ensure vitest configs have `requireAssertions: true`  |
| `testCoverage`      | Check source files have corresponding test files      |
| `goLint`            | Go linting (when enabled)                             |
| `convexValidation`  | Convex schema validation (when enabled)               |
| `buildCheck`        | Build verification (when enabled)                     |

## Configuration

The tool uses a `.pre-commit.json` file in the project root. Create this file to enable and configure checks.

### Configuration File Structure

```json
{
  "packageManager": "pnpm",
  "env": {
    "NODE_OPTIONS": "--max-old-space-size=8192"
  },
  "apps": {
    "web": {
      "path": "apps/web",
      "filter": "@myorg/web",
      "testCommand": "test",
      "nodeMemoryMB": 8192
    },
    "mobile": {
      "path": "apps/mobile",
      "filter": "@myorg/mobile"
    }
  },
  "sharedPaths": ["packages/", "tsconfig.json", ".eslintrc"],
  "reportDir": "./analysis-reports",
  "features": {
    "lintTypecheck": true,
    "lintStaged": true,
    "fullLintOnCommit": false,
    "tests": true,
    "changelog": true,
    "consoleCheck": true,
    "branchProtection": false,
    "goLint": false,
    "convexValidation": false,
    "buildCheck": false,
    "frontendStructure": true,
    "srp": true,
    "fullSRPOnCommit": false,
    "testFiles": true,
    "mockCheck": true,
    "vitestAssertions": true,
    "testCoverage": true
  },
  "protectedBranches": ["main", "production"],
  "changelogExclude": ["^docs/", "^README"],
  "changelog": {
    "mode": "global",
    "globalDir": ".changelog",
    "apps": []
  },
  "consoleAllowed": ["scripts/", "cli/"],
  "typecheckFilter": {
    "errorCodes": ["TS2589", "TS2742"],
    "excludePaths": ["__tests__/", ".test.", ".spec."],
    "skipLibCheck": true,
    "useBuildMode": false
  },
  "lintFilter": {
    "rules": [],
    "excludePaths": ["__tests__/", ".test."],
    "linter": "eslint"
  },
  "lintStagedConfig": {
    "packageManager": "pnpm",
    "env": {
      "COREPACK_ENABLE_STRICT": "0"
    }
  },
  "goLint": {
    "paths": ["./pre-commit", "./other-go-packages"],
    "tool": "golangci-lint"
  },
  "convex": {
    "path": "apps/backend/convex",
    "successMarker": "Convex functions ready!"
  },
  "build": {
    "apps": ["web", "mobile"]
  },
  "mockCheck": {
    "enforceDirectory": true
  },
  "testConfig": {
    "affectedOnly": false,
    "runOnSharedChanges": true,
    "appOverrides": {
      "web": {
        "enabled": true,
        "onlyWhenAffected": true
      },
      "mobile": {
        "enabled": false
      }
    }
  },
  "testCoverageConfig": {
    "appPaths": ["apps/portal", "apps/mobile"],
    "requireTestFolders": [
      "hooks",
      "read",
      "create",
      "update",
      "delete",
      "utils"
    ],
    "excludeFiles": ["index.ts", "*.types.ts"],
    "excludePaths": ["__tests__/", "fixtures/"]
  },
  "srpConfig": {
    "appPaths": ["apps/portal", "apps/mobile"],
    "excludePaths": ["data-layer/", "providers/"],
    "hideWarnings": false
  }
}
```

### Key Configuration Options

#### Global Options

- **packageManager**: `pnpm` (default), `bun`, `npm`, `yarn`
- **env**: Environment variables passed to all commands
- **reportDir**: Directory for detailed analysis reports (organized by check type)

#### Apps Configuration

Each app requires:

- **path**: Filesystem path to the app
- **filter**: Package manager filter name (for `pnpm --filter`)
- **testCommand** (optional): Custom test script name (default: `test`)
- **nodeMemoryMB** (optional): Memory limit for Node.js processes
- **typecheckFilter** (optional): Per-app typecheck overrides

#### Features

Enable/disable checks with boolean flags. Some checks support extended configuration (see sections below).

#### Type Checking Configuration

```json
"typecheckFilter": {
  "errorCodes": ["TS2589", "TS2742"],
  "excludePaths": ["__tests__/", ".test."],
  "skipLibCheck": true,
  "useBuildMode": false
}
```

- **errorCodes**: TypeScript error codes to filter out globally
- **excludePaths**: File paths to exclude from type checking
- **skipLibCheck**: Skip checking `.d.ts` files (default: true for compatibility)
- **useBuildMode**: Use `tsc -b` instead of `tsc --noEmit` (default: false)

#### Linting Configuration

```json
"lintFilter": {
  "rules": [],
  "excludePaths": ["__tests__/"],
  "linter": "eslint"
}
```

- **linter**: `eslint` (default) or `oxlint`
- **rules**: ESLint rule names to filter out
- **excludePaths**: Files to exclude from linting

#### Changelog Configuration

```json
"changelog": {
  "mode": "global|per-app|required",
  "globalDir": ".changelog",
  "apps": []
}
```

- **global**: Single `.changelog/` directory at project root
- **per-app**: Each app has its own `.changelog/`, with fallback to global for shared changes
- **required**: Each affected app must have its own changelog (no global fallback)

#### SRP (Single Responsibility Principle) Configuration

```json
"srpConfig": {
  "appPaths": ["apps/portal", "apps/mobile"],
  "excludePaths": ["data-layer/", "providers/"],
  "hideWarnings": false
}
```

- **appPaths**: Limit SRP checks to specific app directories
- **excludePaths**: Paths to exclude from SRP checking
- **hideWarnings**: Don't show warnings, only errors

#### Test Configuration

```json
"testConfig": {
  "affectedOnly": false,
  "runOnSharedChanges": true,
  "appOverrides": {
    "web": {
      "enabled": true,
      "onlyWhenAffected": true
    }
  }
}
```

- **affectedOnly**: Only run tests for apps with staged changes
- **runOnSharedChanges**: Run all tests when shared paths change (default: true)
- **appOverrides**: Per-app test configuration
  - **enabled**: Override global tests flag for this app
  - **onlyWhenAffected**: Run tests only when this app is affected

## Environment Variables

### Global Environment Variables

Set in the `env` section of config, passed to all commands:

```json
"env": {
  "NODE_OPTIONS": "--max-old-space-size=8192",
  "SOME_API_KEY": "value"
}
```

### Lint-Staged Environment Variables

Set in `lintStagedConfig.env`:

```json
"lintStagedConfig": {
  "env": {
    "COREPACK_ENABLE_STRICT": "0"
  }
}
```

### Pre-commit Specific Variables

- `SKIP_CHANGELOG_CHECK=1` - Skip changelog validation (useful for automated commits)
- `NODE_OPTIONS` - Set Node.js memory limits (e.g., `--max-old-space-size=8192`)

## Exit Codes

- **0** - All checks passed
- **1** - One or more checks failed
  - Check output will show which checks failed
  - Detailed reports written to `--report-dir` if specified

## Example Usage Patterns

### Basic Setup

1. Create `.pre-commit.json`:

```json
{
  "packageManager": "pnpm",
  "apps": {
    "web": {
      "path": "apps/web",
      "filter": "@myorg/web"
    }
  },
  "sharedPaths": ["packages/", "tsconfig.json"],
  "features": {
    "lintTypecheck": true,
    "tests": true
  }
}
```

2. Run checks:

```bash
pre-commit
```

### Monorepo with Multiple Apps

```json
{
  "packageManager": "pnpm",
  "apps": {
    "web": {
      "path": "apps/web",
      "filter": "@myorg/web",
      "nodeMemoryMB": 8192
    },
    "mobile": {
      "path": "apps/mobile",
      "filter": "@myorg/mobile"
    },
    "admin": {
      "path": "apps/admin",
      "filter": "@myorg/admin",
      "testCommand": "test:ci"
    }
  },
  "sharedPaths": ["packages/", "infra/"],
  "features": {
    "lintTypecheck": true,
    "tests": true,
    "changelog": true,
    "srp": true,
    "frontendStructure": true
  }
}
```

### Skip Tests for Specific App

```json
{
  "features": {
    "tests": true
  },
  "testConfig": {
    "appOverrides": {
      "admin": {
        "enabled": false
      }
    }
  }
}
```

### Run Full Checks on Every Commit

```json
{
  "features": {
    "fullLintOnCommit": true,
    "fullSRPOnCommit": true
  }
}
```

### Generate Analysis Reports

```bash
pre-commit --report-dir ./reports
```

Reports are organized by check type:

- `reports/branch-name_timestamp/lint/` - ESLint reports
- `reports/branch-name_timestamp/typecheck/` - TypeScript reports
- `reports/branch-name_timestamp/srp/` - SRP analysis
- `reports/branch-name_timestamp/console-check/` - Console statement violations

## Check Details

### Lint & Type Checking

Runs ESLint and TypeScript checks in parallel for all affected apps.

**Filtering**:

- Filter specific TypeScript error codes (TS2589, TS2742 by default)
- Filter specific ESLint rules
- Exclude specific file patterns

**Modes**:

- **Incremental**: Only check staged files (default)
- **Full**: Check entire app (enabled with `fullLintOnCommit`)

### Tests

Runs test suites based on configuration.

**Behavior**:

- Defaults to all apps unless `affectedOnly` is true
- Can specify custom test command per app
- Skips shared test runs if no shared paths changed

### SRP (Single Responsibility Principle)

Validates code organization and architecture:

- No direct Convex imports outside data-layer
- Screens/pages have no state management
- Single export per file in CRUD folders
- File size limits (screens: 100 lines, hooks: 150 lines, components: 200 lines)
- Types in dedicated `types/` folders
- No mixed concerns (data fetching + UI + state)

### Console Check

Detects `console.log`, `console.warn`, `console.error`, etc.

**Configuration**:

- `consoleAllowed`: File patterns where console statements are permitted (e.g., scripts, CLI)

### Frontend Structure

Validates CRUD folder structure in component directories (create, read, update, delete folders with proper organization).

### Changelog Validation

Enforces changelog entries for commits.

**Modes**:

- **global**: Single `.changelog/` at root
- **per-app**: Each app has `.changelog/`, global fallback for shared changes
- **required**: Each affected app must have changelog (no global fallback)

**Skip temporarily**:

```bash
SKIP_CHANGELOG_CHECK=1 git commit -m "message"
```

### Mock Check

Ensures test files use `__mocks__/` directory for mocks instead of inline `jest.mock()` calls.

### Test Files

Ensures corresponding test files exist for source files.

### Vitest Assertions

Validates that vitest config files have `requireAssertions: true` to prevent empty test suites.

### Go Linting

Runs Go linter on specified paths (golangci-lint by default).

### Convex Validation

Validates Convex backend schema and generates updated type definitions.

### Build Check

Verifies specified apps can build successfully.

## Parallel Execution

The tool runs checks in parallel for performance:

- Multiple apps' lint/typecheck run concurrently
- Tests for different apps run concurrently
- Independent checks run simultaneously

Reports are written sequentially after all jobs complete to avoid race conditions.

## Output

### During Execution

```bash
Running pre-commit checks...

Detected 2 changed file(s) in web app
Detected 1 changed file(s) in mobile app

================================
  LINT & TYPE CHECKING (PARALLEL)
================================

Running checks on 2 app(s) in parallel...

🔍 Running incremental checks for web (5 files)...
   ✓ web passed typecheck
   ✓ web passed lint
✅ web passed all checks

🔍 Running incremental checks for mobile (3 files)...
   ✓ mobile passed typecheck
   ✓ mobile passed lint
✅ mobile passed all checks

================================
  ALL PRE-COMMIT CHECKS PASSED!
================================
```

### On Failure

```text
❌ Found 2 lint error(s)

apps/web/src/Button.tsx
  5:10  error  'unused'  @typescript-eslint/no-unused-vars
  15:5  error  Unexpected any  @typescript-eslint/no-explicit-any

================================
  PRE-COMMIT CHECKS FAILED
================================

Fix the errors above and try again
```

## Performance Optimization

### Tips for Faster Checks

1. **Use incremental checking** (default):
   - Only processes staged files instead of entire app
   - Set `fullLintOnCommit: false`

2. **Configure type checking**:
   - Set `skipLibCheck: true` to skip `.d.ts` file checks
   - Use `useBuildMode: true` only when needed

3. **Memory settings**:
   - Set `nodeMemoryMB` per app if experiencing out-of-memory
   - Example: `"nodeMemoryMB": 8192` for 8GB limit

4. **Run specific checks**:
   - Use `--check` flag to test individual checks during development
   - Example: `pre-commit --check lintTypecheck`

## Troubleshooting

### "pnpm not found"

Make sure pnpm (or your configured package manager) is in your PATH.

### Lint/typecheck timeouts

Set memory limits in config:

```json
{
  "env": {
    "NODE_OPTIONS": "--max-old-space-size=8192"
  }
}
```

Or per-app:

```json
{
  "apps": {
    "web": {
      "path": "apps/web",
      "nodeMemoryMB": 8192
    }
  }
}
```

### Tests skipped unexpectedly

Check your test configuration:

- Is `tests` feature enabled?
- If `affectedOnly: true`, is the app actually affected by staged changes?
- Check per-app overrides in `testConfig.appOverrides`

### Changelog check failing

Make sure:

- Changelog mode is correctly configured
- Changelog directory exists (`.changelog/` by default)
- Fragment files have `.txt` extension

## Integration with Git Hooks

Install as a pre-commit hook:

```bash
# Copy to .git/hooks/
cp pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

Or use a Git hooks manager like `husky`:

```bash
npx husky add .husky/pre-commit "pre-commit"
```

## Related Tools

- **changelog-add**: Add changelog fragments (used with changelog feature)
- **lint-staged**: Automatic code formatting (runs before pre-commit checks)
- **eslint**: JavaScript/TypeScript linting
- **tsc**: TypeScript type checking
- **vitest**: JavaScript test runner
- **pnpm**: Package manager (supports `--filter` for monorepos)
