# Block Git Config Bypass Patterns — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Block AI agents from bypassing git hooks via inline config overrides (`git -c core.hooksPath=/dev/null`) and git environment variables (`GIT_CONFIG_GLOBAL`, etc.).

**Architecture:** Expand the existing `hookBypassPatterns` slice in `cmd/block-destructive-commands/main.go` with new regex patterns. No new files or structural changes. Tests follow the existing table-driven pattern in the test file.

**Tech Stack:** Go, regex, table-driven tests

**Spec:** `docs/superpowers/specs/2026-04-13-block-git-config-bypass-design.md`

---

### Task 1: Add test cases for git -c config override bypass patterns

**Files:**
- Modify: `cmd/block-destructive-commands/main_test.go:236` (insert before the closing `}` of `TestHookBypassPatterns`)

- [ ] **Step 1: Add test cases to `TestHookBypassPatterns`**

Insert these test cases into the `tests` slice in `TestHookBypassPatterns`, after the existing `{"triple bypass", ...}` entry (line 236) and before the closing `}`:

```go
		// === Git -c config override bypasses ===
		{"core.hooksPath /dev/null", "git -c core.hooksPath=/dev/null commit -m 'msg'", true},
		{"core.hooksPath empty", "git -c core.hooksPath= commit -m 'msg'", true},
		{"core.hooksPath with -C flag", "git -C /path -c core.hooksPath=/dev/null commit -m 'msg'", true},
		{"core.hooksPath via config-env", "git --config-env=core.hooksPath=HOOKS_PATH commit -m 'msg'", true},
		{"commit.gpgSign=false", "git -c commit.gpgSign=false commit -m 'msg'", true},
		{"commit.gpgSign=no", "git -c commit.gpgSign=no commit -m 'msg'", true},
		{"commit.gpgsign=0 case insensitive", "git -c commit.gpgsign=0 commit -m 'msg'", true},
		{"commit.gpgSign=off", "git -c commit.gpgSign=off commit -m 'msg'", true},
		{"tag.gpgSign=false", "git -c tag.gpgSign=false tag v1.0", true},
		{"gpg.program override", "git -c gpg.program=/bin/true commit -m 'msg'", true},

		// === Git environment variable config overrides ===
		{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_GLOBAL=/dev/null git commit -m 'msg'", true},
		{"GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_NOSYSTEM=1 git commit -m 'msg'", true},
		{"GIT_CONFIG_SYSTEM", "GIT_CONFIG_SYSTEM=/dev/null git commit -m 'msg'", true},
		{"GIT_DIR override", "GIT_DIR=/tmp/fake git commit -m 'msg'", true},

		// === Safe -c usage (should NOT be blocked) ===
		{"safe core.pager override", "git -c core.pager=cat log --oneline", false},
		{"gpgSign=true (enabling)", "git -c commit.gpgSign=true commit -m 'msg'", false},
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v -run TestHookBypassPatterns ./cmd/block-destructive-commands/`

Expected: FAIL — the new `true` test cases will report `got blocked=false, want blocked=true` because the patterns don't exist yet. The two `false` cases should already pass.

- [ ] **Step 3: Commit failing tests**

```bash
git add cmd/block-destructive-commands/main_test.go
git commit -m "test(block-destructive): add failing tests for git -c config bypass patterns"
```

---

### Task 2: Implement git -c config override bypass patterns

**Files:**
- Modify: `cmd/block-destructive-commands/main.go:321` (append to `hookBypassPatterns` slice, before the closing `}`)

- [ ] **Step 1: Add patterns to `hookBypassPatterns`**

Insert these entries into the `hookBypassPatterns` slice, after the existing `git merge --no-verify` entry (line 320) and before the closing `}`:

```go
	// Git -c config overrides that bypass hooks or signing
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*-c\s+core\.hooksPath\s*=`), name: "git -c core.hooksPath (hook bypass via inline config)"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*--config-env\s*=\s*core\.hooksPath\s*=`), name: "git --config-env=core.hooksPath (hook bypass via config-env)"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*-c\s+commit\.gpgSign\s*=\s*(false|no|off|0)\b`), name: "git -c commit.gpgSign=false (signing bypass)"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*-c\s+tag\.gpgSign\s*=\s*(false|no|off|0)\b`), name: "git -c tag.gpgSign=false (signing bypass)"},
	{regex: regexp.MustCompile(`(?i)\bgit\s+.*-c\s+gpg\.program\s*=`), name: "git -c gpg.program (GPG program override)"},

	// Git environment variables that override config files
	{regex: regexp.MustCompile(`(?i)\bGIT_CONFIG_GLOBAL\s*=`), name: "GIT_CONFIG_GLOBAL (global config override)"},
	{regex: regexp.MustCompile(`(?i)\bGIT_CONFIG_NOSYSTEM\s*=`), name: "GIT_CONFIG_NOSYSTEM (system config bypass)"},
	{regex: regexp.MustCompile(`(?i)\bGIT_CONFIG_SYSTEM\s*=`), name: "GIT_CONFIG_SYSTEM (system config override)"},
	{regex: regexp.MustCompile(`(?i)\bGIT_DIR\s*=`), name: "GIT_DIR (git directory override)"},
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test -v -run TestHookBypassPatterns ./cmd/block-destructive-commands/`

Expected: ALL PASS — including the new test cases from Task 1.

- [ ] **Step 3: Run ALL tests to check for regressions**

Run: `go test -v ./cmd/block-destructive-commands/`

Expected: ALL PASS — no existing tests should break.

- [ ] **Step 4: Commit implementation**

```bash
git add cmd/block-destructive-commands/main.go
git commit -m "feat(block-destructive): block git -c config and env var hook bypass patterns

Block agents from bypassing hooks via:
- git -c core.hooksPath=/dev/null (hook directory override)
- git --config-env=core.hooksPath=... (hook directory via env mapping)
- git -c commit.gpgSign=false (signing bypass)
- git -c tag.gpgSign=false (signing bypass)
- git -c gpg.program=... (GPG program redirect)
- GIT_CONFIG_GLOBAL, GIT_CONFIG_NOSYSTEM, GIT_CONFIG_SYSTEM env vars
- GIT_DIR env var (fake repo with no hooks)"
```

---

### Task 3: Update documentation

**Files:**
- Modify: `docs/block-destructive-commands.md:259` (insert after the "Git flags" section, before "### Framework-Specific Unsafe Deployments")

- [ ] **Step 1: Add new documentation sections**

Insert the following after line 258 (`- Works with git global flags...`) and before line 260 (`### Framework-Specific Unsafe Deployments`):

```markdown

**Git inline config overrides (`-c` flag):**

- `git -c core.hooksPath=...` (disables hooks by overriding hooks directory)
- `git --config-env=core.hooksPath=...` (same bypass via environment variable mapping)
- `git -c commit.gpgSign=false` / `no` / `off` / `0` (disables commit signing)
- `git -c tag.gpgSign=false` / `no` / `off` / `0` (disables tag signing)
- `git -c gpg.program=...` (redirects GPG to a no-op program)
- Works with other git global flags (e.g., `git -C <path> -c core.hooksPath=/dev/null commit`)

**Git environment variable config overrides:**

- `GIT_CONFIG_GLOBAL=` (overrides global config file path)
- `GIT_CONFIG_NOSYSTEM=` (skips reading system config)
- `GIT_CONFIG_SYSTEM=` (overrides system config file path)
- `GIT_DIR=` (overrides git directory, can point to a fake repo with no hooks)

```

- [ ] **Step 2: Commit documentation**

```bash
git add docs/block-destructive-commands.md
git commit -m "docs(block-destructive): document git -c config and env var bypass patterns"
```

---

### Task 4: Build and verify

- [ ] **Step 1: Build the binary**

Run: `just block-destructive-commands`

Expected: Binary built to `bin/block-destructive-commands` with no errors.

- [ ] **Step 2: Smoke test the binary with the exact attack command from the spec**

Run:
```bash
echo '{"tool_input":{"command":"git -c core.hooksPath=/dev/null commit -m test"}}' | ./bin/block-destructive-commands
echo "Exit code: $?"
```

Expected: Exit code 2, stdout contains JSON with `"permissionDecision":"deny"`, stderr contains `BLOCKED: git -c core.hooksPath`.

- [ ] **Step 3: Smoke test a safe command still passes**

Run:
```bash
echo '{"tool_input":{"command":"git -c core.pager=cat log --oneline"}}' | ./bin/block-destructive-commands
echo "Exit code: $?"
```

Expected: Exit code 0, no output.

- [ ] **Step 4: Run benchmarks to verify no performance regression**

Run: `go test -bench=BenchmarkPatternMatching -benchmem ./cmd/block-destructive-commands/`

Expected: Performance within same order of magnitude as before (nanosecond-range per operation).

- [ ] **Step 5: Final commit (if any fixes needed)**

Only if previous steps revealed issues. Otherwise, skip.
