# Claude Hooks

A collection of hooks and tools for [Claude Code](https://claude.ai/claude-code) that enforce code quality, prevent destructive operations, and automate common tasks.

## Tools

| Tool | Description |
|------|-------------|
| [block-destructive-commands](docs/block-destructive-commands.md) | Prevents dangerous CLI commands (rm -rf, git reset --hard, etc.) and blocks destructive git operations |
| [block-infrastructure](docs/block-infrastructure.md) | Protects critical config files from modification |
| [block-lint-workarounds](docs/block-lint-workarounds.md) | Catches underscore prefixes and suppression comments |
| [changelog-add](docs/changelog-add.md) | Creates changelog fragments with conventional commit format |
| [convex-gen](docs/convex-gen.md) | Generates React hooks and TypeScript types from Convex functions |
| [docs-tracker](docs/docs-tracker.md) | Enforces documentation reading before code edits |
| [enforce-tests-on-commit](docs/enforce-tests-on-commit.md) | Requires tests for modified source files |
| [markdown-formatter](docs/markdown-formatter.md) | Auto-formats markdown with code fence language tags |
| [pre-commit](docs/pre-commit.md) | Orchestrates all pre-commit validation checks |
| [smart-lint](docs/smart-lint.md) | Runs appropriate linters based on project type |
| [smart-test](docs/smart-test.md) | Runs relevant tests after file modifications |
| [track-edited-files](docs/track-edited-files.md) | Tracks source/test file edits per session |
| [validate-frontend-structure](docs/validate-frontend-structure.md) | Enforces CRUD folder organization |
| [validate-srp](docs/validate-srp.md) | Validates Single Responsibility Principle |
| [validate-test-files](docs/validate-test-files.md) | Ensures components have required tests |

## Installation

### From Source

```bash
git clone https://github.com/milehighideas/claude-hooks.git
cd claude-hooks
make build
```

Binaries will be built to the `bin/` directory.

### Install to /usr/local/bin

```bash
make install
```

### Install Individual Tools

```bash
go install github.com/milehighideas/claude-hooks/cmd/smart-lint@latest
```

## Configuration

Add hooks to your Claude Code settings (`~/.claude/settings.json`):

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/claude-hooks/bin/block-destructive-commands"
          }
        ]
      }
    ]
  }
}
```

## Development

```bash
# Run all tests
make test

# Build specific tool
make build-smart-lint

# Clean build artifacts
make clean
```

## License

MIT
