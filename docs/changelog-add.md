# changelog-add

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/changelog-add`)

A command-line utility that creates changelog fragment files with conventional commit formatting.

## Overview

`changelog-add` is a tool for generating changelog entries in a monorepo environment. It creates standardized changelog fragments based on conventional commit messages, supporting three different organization modes: global (all fragments in root `.changelog/`), per-app (fragments routed to app-specific `.changelog/` directories), and required (scope must match a configured app).

The tool integrates with your project's `.pre-commit.json` configuration to determine changelog organization and available apps, making it ideal for teams using conventional commits and semantic versioning.

## Features

- **Multiple organization modes** for flexible changelog management:
  - `global` - All entries to root `.changelog/`
  - `per-app` - Route entries by scope to app-specific directories
  - `required` - Scope must match a configured app, error otherwise
- **Conventional Commit support** - Validates commit format `type(scope): description`
- **Valid commit types** - `feat`, `fix`, `chore`, `docs`, `test`, `style`, `refactor`, `perf`, `build`, `ci`, `revert`
- **Timestamp-based filenames** - Unique fragment filenames with type and scope information
- **Scope-to-app resolution** - Automatically routes entries to app directories based on conventional commit scope
- **Explicit app selection** - Override automatic scope detection with `--app` flag
- **Monorepo awareness** - Finds project root by detecting `.pre-commit.json`, `pnpm-workspace.yaml`, or `package.json`
- **Git-friendly** - Creates `.gitkeep` files to preserve empty `.changelog/` directories
- **Clear error messages** - Helpful validation feedback with examples and available options

## Usage

### Installation

Build the binary:

```bash
cd /Volumes/Developer/code/shared/claude-hooks/changelog-add
go build -o changelog-add
```

Optionally, install to a directory in your PATH:

```bash
go build -o changelog-add
sudo install changelog-add /usr/local/bin/
```

### Basic Usage

Create a changelog entry with the simplest command:

```bash
changelog-add 'feat(native): add login functionality'
```

### Command Line Arguments

```text
changelog-add [--app <name>] [--global] [--list] [--help] 'type(scope): description'
```

#### Positional Arguments

- **entry** - The changelog entry in conventional commit format. Required unless using `--list` or `--help`.
  - Format: `type(scope): description` or `type: description`
  - Must be quoted if it contains spaces

#### Flags

- **`--app <name>`** - Explicitly specify which app to use. Overrides scope-based detection.
  - Example: `changelog-add --app backend 'chore: update dependencies'`
  - Errors if app name doesn't exist in `.pre-commit.json`

- **`--global`** - Force creation in root `.changelog/` directory, ignoring configured mode.
  - Useful in required mode to create entries that aren't app-specific
  - Example: `changelog-add --global 'chore: update CI workflows'`

- **`--list`** - List all available apps configured in `.pre-commit.json` and the current changelog mode.
  - Example: `changelog-add --list`

- **`--help`, `-h`** - Display usage information and available apps.
  - Example: `changelog-add --help`

## Configuration

### .pre-commit.json Configuration

Configure changelog behavior in your project's `.pre-commit.json`:

```json
{
  "apps": {
    "native": {
      "path": "apps/native",
      "filter": "native"
    },
    "web": {
      "path": "apps/web",
      "filter": "@myapp/web"
    },
    "backend": {
      "path": "packages/backend",
      "filter": "@myapp/backend"
    }
  },
  "changelog": {
    "mode": "per-app",
    "apps": ["native", "web", "backend"]
  }
}
```

#### Changelog Configuration Fields

- **`mode`** (string, optional) - How changelogs are organized:
  - `"global"` (default) - All fragments go to root `.changelog/`
  - `"per-app"` - Route fragments by scope to app-specific `.changelog/` directories
  - `"required"` - Scope must match a configured app, error if no match

- **`apps`** (array of strings, optional) - Which apps support changelogs (defaults to all configured apps):
  - Restricts which apps accept changelog entries
  - Useful if some apps don't use changelog fragments

## Changelog Organization Modes

### Global Mode (default)

All entries go to the root `.changelog/` directory:

```bash
changelog-add 'feat(native): add login'
changelog-add 'fix(web): resolve bug'

# Both create fragments in .changelog/
```

Useful for simple projects or monorepos with shared changelog.

### Per-App Mode

Entries are routed to app-specific directories based on scope:

```bash
changelog-add 'feat(native): add login'  # → apps/native/.changelog/
changelog-add 'fix(web): resolve bug'    # → apps/web/.changelog/
changelog-add 'chore: update CI'         # → .changelog/ (no scope)
```

When scope doesn't match an app, falls back to root `.changelog/`. Useful for monorepos where each app has its own changelog.

### Required Mode

Scope must match a configured app, or the command errors:

```bash
changelog-add 'feat(native): add login'  # → apps/native/.changelog/
changelog-add 'fix(web): resolve bug'    # → apps/web/.changelog/
changelog-add 'chore: update CI'         # ERROR: scope required, doesn't match any app

# Must use --app flag or include matching scope
changelog-add --app backend 'chore: update deps'  # Works
```

Useful for strict monorepos where all changes must be attributed to an app.

## Conventional Commit Format

The tool follows the conventional commit specification:

```text
type(scope): description
```

### Valid Types

- `feat` - A new feature
- `fix` - A bug fix
- `chore` - Changes that don't affect code meaning (dependencies, CI, etc.)
- `docs` - Documentation only changes
- `test` - Adding or updating tests
- `style` - Code formatting (whitespace, semicolons, etc.)
- `refactor` - Code refactoring without feature or bug fix
- `perf` - Performance improvements
- `build` - Changes to build system or dependencies
- `ci` - Changes to CI/CD configuration
- `revert` - Reverting a previous commit

### Scope (optional)

A scope is optional but helpful for routing in per-app or required modes:

```bash
feat(native): ...     # Scope: native
fix: resolve issue    # No scope
```

Scopes are matched case-insensitively to app names.

### Description

A brief description of the change. Should be lowercase and imperative mood (e.g., "add", not "adds"):

```bash
feat(web): add user authentication
fix(api): handle concurrent requests
```

## Fragment File Naming

Fragment files are created with timestamp-based names:

```text
YYYYMMDD-HHMMSS-type-scope-description.txt
YYYYMMDD-HHMMSS-type-description.txt  (no scope)
```

Examples:

```text
20250128-154532-feat-native-add-login.txt
20250128-154533-fix-web-resolve-bug.txt
20250128-154534-chore-update-dependencies.txt
```

The description is slugified (lowercase, hyphens for spaces/punctuation, max 50 chars) and the scope is slugified to max 20 chars.

## Environment Variables

The tool does not require any environment variables. Configuration is entirely through `.pre-commit.json` and command-line flags.

## Exit Codes

- **0** - Success. Fragment created and displayed.
- **1** - Error. Invalid input, missing configuration, or file I/O errors.

## Example Usage

### Example 1: Simple Global Mode

```bash
$ changelog-add 'feat(auth): add passwordless login'
Created changelog fragment: .changelog/20250128-154532-feat-auth-add-passwordless-login.txt
   Entry: feat(auth): add passwordless login
```

### Example 2: Per-App Mode with Scope Detection

With per-app mode configured:

```bash
$ changelog-add 'fix(native): resolve memory leak'
Created changelog fragment: apps/native/.changelog/20250128-154533-fix-native-resolve-memory-leak.txt
   Entry: fix(native): resolve memory leak
   App: native
```

### Example 3: Explicit App Override

```bash
$ changelog-add --app backend 'chore: update typescript'
Created changelog fragment: packages/backend/.changelog/20250128-154534-chore-update-typescript.txt
   Entry: chore: update typescript
   App: backend
```

### Example 4: Global Flag Override

In required mode, force root changelog:

```bash
$ changelog-add --global 'chore: update CI workflows'
Created changelog fragment: .changelog/20250128-154535-chore-update-ci-workflows.txt
   Entry: chore: update CI workflows
```

### Example 5: List Available Apps

```bash
$ changelog-add --list
Changelog mode: per-app
Available apps:
  backend (packages/backend)
  native (apps/native)
  web (apps/web)
```

### Example 6: Error Cases

Invalid format:

```bash
$ changelog-add 'add some feature'
Error: invalid format. Expected: 'type(scope): description' or 'type: description'

Examples:
  feat(native): add login functionality
  fix(web): resolve navigation bug
  chore(backend): update dependencies
```

Invalid type:

```bash
$ changelog-add 'feature(app): add something'
Error: invalid type 'feature'. Valid types: build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test
```

Required mode without app:

```bash
$ changelog-add 'chore: update dependencies'
Error: mode 'required' requires scope to match an app
Scope '' doesn't match any configured app

Available apps:
  backend
  native
  web
```

## How It Works

The tool executes the following process:

1. **Startup**: Finds project root by searching for `.pre-commit.json`, `pnpm-workspace.yaml`, or `package.json`
2. **Configuration**: Loads `.pre-commit.json` and extracts changelog mode and app configuration
3. **Parsing**: Parses the entry for conventional commit format (type, scope, description)
4. **Validation**: Validates that type is in the allowed list
5. **App Resolution**: Based on mode and scope, determines which app directory to use
6. **Directory Creation**: Creates `.changelog/` directory and `.gitkeep` file if needed
7. **Fragment Writing**: Creates the fragment file with timestamp-based name and entry content
8. **Output**: Reports the relative path, entry text, and app name (if applicable)

## Project Root Detection

The tool searches upward from the current directory for project root markers:

1. `.pre-commit.json` (preferred)
2. `pnpm-workspace.yaml` (monorepo marker)
3. `package.json` (Node.js project marker)

Returns the first match found. If none exist, uses the current working directory.

## Testing

Run tests with:

```bash
cd /Volumes/Developer/code/shared/claude-hooks/changelog-add
go test -v
```

Test coverage includes:

- Conventional commit format parsing (with and without scope)
- Filename sanitization and slugification
- App resolution and scope matching
- All three changelog modes (global, per-app, required)
- Configuration loading from `.pre-commit.json`
- Fragment file creation and git handling
- Error messages and validation

## Integration with Claude Code Hooks

This tool is designed to be used directly from the command line or integrated into CI/CD pipelines. It works well with the `claude-hooks` suite for managing changelog fragments:

- Use `changelog-add` to create individual fragments
- Combine with other tools like `smart-lint` and `enforce-tests-on-commit`
- Automate in pre-commit hooks or CI workflows

## Related Tools

This tool is part of the claude-hooks suite:

- `smart-test` - Intelligent testing on file edits
- `smart-lint` - Intelligent linting on file edits
- `enforce-tests-on-commit` - Git pre-commit test enforcement
