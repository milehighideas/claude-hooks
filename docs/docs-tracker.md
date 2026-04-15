# docs-tracker

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/docs-tracker`)

A Go binary that gates Edit/Write operations on reading required documentation, and tracks documentation reads so the gate clears once a doc has been read.

## Operational modes

1. **Enforce Mode** (PreToolUse hook): Blocks Edit/Write if any required doc for the target path hasn't been read this session.
2. **Track Mode** (PostToolUse hook): Records when a registered doc is read so enforce mode will allow subsequent edits.

## Opt-in per project

The hook only activates for projects that contain a `.pre-commit.json` with the `docsTracker` feature enabled:

```jsonc
// <project-root>/.pre-commit.json
{
  "features": {
    "docsTracker": true
  }
}
```

Absent `.pre-commit.json`, or with `features.docsTracker` unset / false, the hook is a silent no-op — safe to wire into `~/.claude/settings.json` globally. JSONC (`//` comments) is supported so the file can be annotated alongside the other feature flags consumed by `pre-commit` and `validate-test-files`.

At invocation the binary walks up from the tool's `file_path` to find the nearest directory containing `.pre-commit.json`, treating that as the project root.

## Config

Settings live under `docsTrackerConfig`:

```jsonc
{
  "features": { "docsTracker": true },
  "docsTrackerConfig": {
    "autoDiscover": true,
    "docFileNames": ["CLAUDE.md"],
    "convex": false,
    "mappings": [],
    "appPaths": [],
    "excludePaths": []
  }
}
```

| Field | Type | Default | Description |
| --- | --- | --- | --- |
| `autoDiscover` | bool | `true` | Walk the project for files named in `docFileNames` and build one mapping per directory. |
| `docFileNames` | string[] | `["CLAUDE.md"]` | File names that auto-discovery treats as required docs for their containing directory. |
| `convex` | bool \| object | `false` | Enables the Convex preset. See [Convex preset](#convex-preset). |
| `mappings` | object[] | `[]` | Explicit directory-to-docs rules. See [Custom mappings](#custom-mappings). |
| `appPaths` | string[] | `[]` | Restricts enforcement to files whose project-relative path contains at least one of these substrings. Empty = everything in scope. |
| `excludePaths` | string[] | `[]` | Skips enforcement on files whose project-relative path contains any of these substrings. Exclusions always win over `appPaths`. |

Unknown fields are ignored. `appPaths` / `excludePaths` mirror the shape of `srpConfig`, `testCoverageConfig`, and `testFilesConfig` elsewhere in `.pre-commit.json`.

## Custom mappings

Escape hatch for cases where the required doc lives outside the gated directory, or where you want a fixed path regardless of file name. Each mapping declares a directory `pattern` and the `docs` that must be read before editing inside it.

```jsonc
{
  "features": { "docsTracker": true },
  "docsTrackerConfig": {
    "mappings": [
      {
        "pattern": "apps/web/",
        "docs": ["docs/frontend-architecture.md", "CONTRIBUTING.md"]
      },
      {
        "pattern": "packages/backend/",
        "docs": ["docs/api.md"],
        "name": "Backend API"
      }
    ]
  }
}
```

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `pattern` | string | yes | Project-relative directory prefix that triggers the gate. Leading `/` is stripped; trailing `/` is added if absent. |
| `docs` | string[] | yes | Project-relative paths to required docs. All must be read before edits in `pattern` are allowed. |
| `name` | string | no | Friendly label used in the block message. Defaults to `pattern` without the trailing slash. |

### Precedence with other mapping sources

Custom mappings coexist with the Convex preset and auto-discovery:

- **Preset + custom on the same pattern** → docs are merged (union). Editing inside the preset's backend dir requires both the preset docs and any custom docs you added.
- **Custom + auto-discovery on the same pattern** → docs are merged. Dropping `apps/web/CLAUDE.md` while also declaring a custom mapping for `apps/web/` means both must be read.
- **Most-specific pattern wins** when multiple patterns match a file: `apps/web/components/` takes precedence over `apps/web/`, regardless of source.

### When to use custom mappings vs. `docFileNames`

- **Use `docFileNames`** when the doc lives *inside* the directory it describes (e.g., `apps/web/CLAUDE.md`). Auto-discovery handles it automatically.
- **Use `mappings`** when the doc lives *elsewhere* (root-level `docs/`, a sibling directory), or when you want a specific file regardless of its name.

### Per-app scope example

Monorepo where only two apps should gate on CLAUDE.md:

```jsonc
{
  "features": { "docsTracker": true },
  "docsTrackerConfig": {
    "appPaths": ["apps/web", "packages/ui"],
    "excludePaths": ["apps/web/legacy"]
  }
}
```

Edits inside `apps/mobile/` are never gated. Edits inside `apps/web/legacy/` are never gated even though `apps/web` is in scope.

## Migrating from `.claude/docs-tracker.json`

Earlier versions used a standalone `.claude/docs-tracker.json` marker. Move your settings into `.pre-commit.json`:

```jsonc
// before — .claude/docs-tracker.json
{ "convex": true }
```

```jsonc
// after — .pre-commit.json
{
  "features": { "docsTracker": true },
  "docsTrackerConfig": { "convex": true }
}
```

Delete the old `.claude/docs-tracker.json` once migrated — the binary no longer reads it.

## How mappings are built

For each Edit/Write the binary:

1. Finds the project root via the opt-in marker.
2. Bails out unless `features.docsTracker` is true.
3. Applies any **skip patterns** to short-circuit for tests/generated files.
4. Applies the **per-app scope** (`appPaths` / `excludePaths`); out-of-scope files are silent no-ops.
5. Builds a list of mappings, merging docs per pattern from all three sources:
   - If the **Convex preset** is enabled, adds a mapping for the backend dir.
   - Adds any **custom `mappings`** from the config.
   - If **`autoDiscover`** is true, walks the project for `docFileNames` and adds a mapping per directory.

   When multiple sources produce a mapping for the same pattern, their docs are combined (union, deduplicated). To have the preset authoritatively replace auto-discovery in its subtree, set `autoDiscover: false`.
6. Sorts longest-pattern-first so the most specific mapping wins.
7. Checks whether every doc in the matched mapping has been read this session.

### Auto-discovery rules

- The **root-level doc file** is ignored (Claude Code already loads a root CLAUDE.md as context).
- These directories are **never walked**: `node_modules`, `.git`, `dist`, `build`, `.next`, `.turbo`, `.vercel`, `_generated`.
- When multiple matching doc file names are present in a single directory, **all** are required reading.

### Skip patterns

These path fragments bypass enforcement entirely (no doc required to edit them):

- `__tests__/`, `.test.ts`, `.test.tsx`
- `_generated/`, `.d.ts`
- `node_modules/`

Additionally, the binary will not block editing of a file that is itself one of the required docs for its matched mapping — you can update the doc you'd otherwise be gated on.

## Convex preset

Convex projects share a consistent generated layout; the preset encodes it.

```jsonc
{
  "features": { "docsTracker": true },
  "docsTrackerConfig": { "convex": true }
}
```

Or with a custom backend location:

```jsonc
{
  "features": { "docsTracker": true },
  "docsTrackerConfig": {
    "convex": { "backendDir": "apps/backend" }
  }
}
```

With the preset enabled:

- **Pattern**: `<backendDir>/` (default `packages/backend/`).
- **Required docs**:
  - `<backendDir>/convex/_generated/ai/guidelines.md` (if it exists)
  - `<backendDir>/.agents/skills/*/SKILL.md` (glob-expanded at invocation time)
- **Name** in blocked-message: `Convex backend (<backendDir>)`.
- **Scope**: only edits under `<backendDir>/` are gated. Files elsewhere are unaffected by the preset.
- **Coexistence**: the preset merges with auto-discovery and custom mappings for the same pattern. If Convex installed a `CLAUDE.md` alongside the skills (via `npx convex ai-files install`), it is required in addition to the preset's guidelines + SKILL.md files. Set `autoDiscover: false` to have the preset be authoritative and suppress the CLAUDE.md requirement.

If `<backendDir>` doesn't exist or has no resolvable docs, the preset is silent (no mapping created).

## Command-line usage

```bash
docs-tracker -mode enforce < hook_input.json
docs-tracker -mode track   < hook_input.json
```

### Input format

```json
{
  "tool_name": "Edit",
  "tool_input": { "file_path": "/abs/path/to/file.ts" },
  "session_id": "unique-session-id"
}
```

`file_path` may be absolute or relative; relative paths are resolved against cwd.

### Exit codes

| Mode | Code | Meaning |
| --- | --- | --- |
| enforce | `0` | Allowed (no config, no mapping, skip pattern, or all docs read). |
| enforce | `1` | System error. |
| enforce | `2` | Blocked — at least one required doc unread. |
| track   | `0` | Success (including no-op cases). |
| track   | `1` | System error (couldn't write session state). |

## Blocked output

```text
⚠️  PLEASE READ DOCUMENTATION FIRST

Before editing files in Convex backend (packages/backend), please read the following:
  packages/backend/convex/_generated/ai/guidelines.md
  packages/backend/.agents/skills/convex-quickstart/SKILL.md
  packages/backend/.agents/skills/convex-setup-auth/SKILL.md

This ensures you follow project conventions and patterns.

Retry your edit once the documentation has been read.
```

Only **unread** docs are listed. Once every doc in the mapping has been read in the session, subsequent edits are allowed.

## Session storage

```text
~/.claude/sessions/{session_id}-docs.json
```

```json
{ "docs_read": ["packages/backend/convex/_generated/ai/guidelines.md"] }
```

Paths are stored **relative to the project root** so enforce and track share the same keys regardless of how Claude Code expresses the file path.

## Graceful degradation

The tool fails open on unrecoverable issues:

- Invalid JSON input → allow.
- Unreadable or malformed `.pre-commit.json` → allow.
- Missing session file → allow (first session, nothing tracked yet).
- Session file I/O error → allow.
- No project root → allow.

## Wiring into Claude Code

Register the binary globally in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [{ "type": "command", "command": "/path/to/bin/docs-tracker -mode enforce" }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Read",
        "hooks": [{ "type": "command", "command": "/path/to/bin/docs-tracker -mode track" }]
      }
    ]
  }
}
```

## Recipes

### Monorepo with per-package CLAUDE.md

```json
{}
```

Walks the project and requires reading the corresponding `CLAUDE.md` before editing inside each package/app directory.

### Convex backend only, no CLAUDE.md elsewhere

```json
{
  "autoDiscover": false,
  "convex": true
}
```

Gates edits under `packages/backend/` on the generated guidelines and installed SKILL.md files. Everywhere else: no gating.

### Convex backend + CLAUDE.md for everything else

```json
{
  "convex": true
}
```

The preset claims `packages/backend/`; auto-discovery handles every other directory that has a `CLAUDE.md`.

### Use AGENTS.md instead of CLAUDE.md

```json
{
  "docFileNames": ["AGENTS.md"]
}
```

### Both AGENTS.md and CLAUDE.md required

```json
{
  "docFileNames": ["CLAUDE.md", "AGENTS.md"]
}
```

When a directory has both, both must be read.

## Building

```bash
just docs-tracker
```

Produces `bin/docs-tracker`.

## Testing

```bash
go test ./cmd/docs-tracker/...
```

Tests cover:

- Opt-in gating (no config → no-op for both modes).
- Auto-discovery (ordering, ignored dirs, custom `docFileNames`).
- Convex preset (block/allow, missing-only messaging, custom `backendDir`, scope, editing own docs).
- Preset + auto-discovery coexistence.
- Skip patterns and non-Edit tool bypass.
- Track registration, deduplication, and unregistered-doc rejection.
- Session persistence round-trip.
- Config unmarshaling (bare `true/false` and object forms for `convex`).
