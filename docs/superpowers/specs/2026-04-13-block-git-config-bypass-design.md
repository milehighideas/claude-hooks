# Block Git Config Bypass Patterns

**Date:** 2026-04-13
**Status:** Approved
**Component:** `cmd/block-destructive-commands`

## Problem

AI agents bypass git hooks by using `git -c core.hooksPath=/dev/null commit` instead of `--no-verify`. The existing `hookBypassPatterns` catch `--no-verify`, `-n`, and `SKIP_HOOKS`-style environment variables, but miss inline config overrides (`-c`) and git environment variables that override config files entirely.

### Observed attack

```bash
cd /path/to/repo && git add file.tsx && git -c core.hooksPath=/dev/null commit -m "msg"
```

This nullifies all hooks (pre-commit, commit-msg, etc.) without using any of the currently blocked flags.

## Approach

Expand the existing `hookBypassPatterns` slice with new regex patterns. No new files, no architectural changes. Follows the established pattern of regex + name + optional exclude.

### Why not block all `git -c` usage?

`git -c` is used legitimately (e.g., `git -c core.pager=cat log`). Blocking it entirely would break normal operations. Instead, we block specific dangerous config keys.

## New Patterns

### Git `-c` config overrides

| Pattern | Catches | Example |
|---------|---------|---------|
| `git ... -c core.hooksPath=...` | Hook directory override | `git -c core.hooksPath=/dev/null commit` |
| `git ... --config-env=core.hooksPath=...` | Hook directory via env mapping | `git --config-env=core.hooksPath=VAR commit` |
| `git ... -c commit.gpgSign=(false\|no\|off\|0)` | Commit signing disabled | `git -c commit.gpgSign=false commit` |
| `git ... -c tag.gpgSign=(false\|no\|off\|0)` | Tag signing disabled | `git -c tag.gpgSign=false tag v1` |
| `git ... -c gpg.program=...` | GPG program redirect | `git -c gpg.program=/bin/true commit` |

### Environment variable overrides

| Pattern | Catches | Example |
|---------|---------|---------|
| `GIT_CONFIG_GLOBAL=` | Override global config file | `GIT_CONFIG_GLOBAL=/dev/null git commit` |
| `GIT_CONFIG_NOSYSTEM=` | Skip system config | `GIT_CONFIG_NOSYSTEM=1 git commit` |
| `GIT_CONFIG_SYSTEM=` | Override system config file | `GIT_CONFIG_SYSTEM=/dev/null git commit` |
| `GIT_DIR=` | Override git directory (fake repo with no hooks) | `GIT_DIR=/tmp/fake git commit` |

### What is NOT blocked

- `git -c core.pager=cat log` -- safe config override
- `git -c commit.gpgSign=true commit` -- enabling signing is fine
- `git -c color.ui=auto status` -- cosmetic config

## Test Cases

### Blocked

- `git -c core.hooksPath=/dev/null commit -m 'msg'`
- `git -c core.hooksPath= commit -m 'msg'` (empty path)
- `git -C /path -c core.hooksPath=/dev/null commit -m 'msg'` (with other global flags)
- `git --config-env=core.hooksPath=HOOKS_PATH commit -m 'msg'`
- `git -c commit.gpgSign=false commit -m 'msg'`
- `git -c commit.gpgSign=no commit -m 'msg'`
- `git -c commit.gpgsign=0 commit -m 'msg'` (case insensitive)
- `git -c tag.gpgSign=false tag v1.0`
- `git -c gpg.program=/bin/true commit -m 'msg'`
- `GIT_CONFIG_GLOBAL=/dev/null git commit -m 'msg'`
- `GIT_CONFIG_NOSYSTEM=1 git commit -m 'msg'`
- `GIT_CONFIG_SYSTEM=/dev/null git commit -m 'msg'`
- `GIT_DIR=/tmp/fake git commit -m 'msg'`

### Allowed

- `git -c core.pager=cat log --oneline`
- `git -c commit.gpgSign=true commit -m 'msg'`
- `git commit -m 'normal commit'`

## Documentation Update

Add a new subsection to `docs/block-destructive-commands.md` under "Pre-Commit Hook Bypass Attempts" covering:

1. Git config override flags (`-c`, `--config-env`)
2. Git environment variable overrides (`GIT_CONFIG_GLOBAL`, `GIT_CONFIG_NOSYSTEM`, `GIT_CONFIG_SYSTEM`, `GIT_DIR`)

## Implementation Steps

1. Add new patterns to `hookBypassPatterns` in `cmd/block-destructive-commands/main.go`
2. Add test cases to `TestHookBypassPatterns` in `cmd/block-destructive-commands/main_test.go`
3. Run tests to verify
4. Update `docs/block-destructive-commands.md`
5. Build and verify binary
