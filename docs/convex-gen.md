# convex-gen

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/convex-gen`)

A code generation tool that automatically creates a TypeScript data layer for Convex backend applications. It scans your Convex schema and functions, then generates React hooks, API wrappers, and type definitions.

## Overview

`convex-gen` is a command-line tool designed to eliminate boilerplate in Convex-based applications. It generates three categories of output:

1. **Hooks** - React hooks for queries, mutations, and actions
2. **API Wrappers** - Type-safe API references organized by function type
3. **Types** - TypeScript types derived from your Convex schema

## Usage

### As a CLI

```bash
convex-gen
```

Run the tool from the root of your project (where `.convex-gen.json` exists). The tool will:

1. Load configuration from `.convex-gen.json` or `convex-gen.json`
2. Scan Convex functions and schema files
3. Parse function signatures and table definitions
4. Generate output files in the configured directories

### As a Claude Hook

This tool is designed to be used as part of the claude-hooks system. You can invoke it through your development workflow to automatically keep generated code in sync with your Convex backend.

## Configuration

### `.convex-gen.json` Structure

```json
{
  "org": "@organization",
  "convex": {
    "path": "packages/backend",
    "schemaPath": "packages/backend/schema",
    "structure": "nested"
  },
  "dataLayer": {
    "path": "packages/data-layer/src",
    "hooksDir": "generated-hooks",
    "apiDir": "generated-api",
    "typesDir": "generated-types",
    "fileStructure": "grouped"
  },
  "imports": {
    "style": "package",
    "api": "@organization/backend/api",
    "dataModel": "@organization/backend/dataModel"
  },
  "generators": {
    "hooks": true,
    "api": true,
    "types": true
  },
  "skip": {
    "directories": ["_generated", "node_modules", ".turbo"],
    "patterns": [
      "^_",
      "\\.test\\.",
      "\\.spec\\.",
      "^debug",
      "^migrate",
      "^seed"
    ]
  }
}
```

### Configuration Options

#### `org` (required)

- Organization name (e.g., `"@dashtag"`)
- Used for import path generation with package-style imports

#### `convex` object

- **`path`** - Path to Convex backend directory (default: `"packages/backend"`)
- **`schemaPath`** - Path to schema file or directory (default: auto-detected)
- **`structure`** - Directory structure: `"nested"` or `"flat"` (default: `"nested"`)

#### `dataLayer` object

- **`path`** - Root output directory for generated code (default: `"packages/data-layer/src"`)
- **`hooksDir`** - Subdirectory for hooks (default: `"generated-hooks"`)
- **`apiDir`** - Subdirectory for API wrappers (default: `"generated-api"`)
- **`typesDir`** - Subdirectory for types (default: `"generated-types"`)
- **`fileStructure`** - Output structure: `"grouped"`, `"split"`, or `"both"` (default: `"grouped"`)

#### `imports` object

- **`style`** - Import style: `"package"` (recommended) or `"relative"` (default: `"package"`)
- **`api`** - Import path for Convex API (default: auto-calculated)
- **`dataModel`** - Import path for Convex types (default: auto-calculated)

#### `generators` object

- **`hooks`** - Generate React hooks (default: `true`)
- **`api`** - Generate API wrappers (default: `true`)
- **`types`** - Generate schema types (default: `true`)

#### `skip` object

- **`directories`** - Directory names to skip during scanning
- **`patterns`** - Regex patterns for files to skip

### File Structure Options

#### `grouped` (default)

One file per top-level namespace. Example:

```text
generated-hooks/
├── queries/
│   ├── useEvents.ts      # All events queries
│   └── index.ts
├── mutations/
│   ├── useEvents.ts      # All events mutations
│   └── index.ts
└── actions/
```

#### `split`

One file per full namespace (sub-namespace). Example:

```text
generated-hooks/
├── queries/
│   ├── useEvents_voting.ts
│   ├── useEvents_attendees.ts
│   └── index.ts
├── mutations/
│   └── ...
└── actions/
```

#### `both`

Generates both grouped and split file structures.

### Import Styles

#### `package` (recommended)

Uses npm/yarn package imports:

```typescript
import { api } from "@organization/backend/api";
import type { Id } from "@organization/backend/dataModel";
```

#### `relative`

Uses relative paths:

```typescript
import { api } from "../../../backend/_generated/api";
import type { Id } from "../../../backend/_generated/dataModel";
```

## Generated Output

### Hooks (`generators.hooks: true`)

Generated React hooks for querying, mutating, and calling Convex functions.

**Features:**

- Typed parameters with null safety
- Conditional query skip support
- Paginated query support
- Automatic `shouldSkip` parameter for queries without required arguments

**Example output:**

```typescript
import { useQuery } from "convex/react";
import { api } from "@organization/backend/api";
import type { Id } from "@organization/backend/dataModel";

/**
 * Hook to get event by id
 *
 * @param eventId - ID of events
 */
export function useEventsGetEventById(
  eventId: Id<"events"> | null | undefined,
  shouldSkip?: boolean,
) {
  return useQuery(eventId ? ({ eventId } as any) : "skip");
}
```

#### Requiring `shouldSkip` on auth-gated queries (`dataLayer.requireAuthGatedShouldSkip`)

By default, `shouldSkip` is always optional — a caller can simply not pass it,
and the query still runs. That's fine for public queries, but for a query
whose backend handler requires authentication (throws if unauthenticated),
an omitted `shouldSkip` means the hook fires during the window before the
client has finished authenticating, producing a runtime error instead of a
silent skip.

Set `dataLayer.requireAuthGatedShouldSkip: true` and `dataLayer.authHelperNames`
(defaults to `["getAuthenticatedUser", "getAuthenticatedUserForActions"]`) to
make `shouldSkip` a **required, compiler-enforced** param on any query hook
whose backend handler calls one of those helpers — every caller must now
explicitly reckon with the unauthenticated case (e.g. `shouldSkip={!isSignedIn}`)
or the build fails.

```json
{
  "dataLayer": {
    "requireAuthGatedShouldSkip": true,
    "authHelperNames": ["getAuthenticatedUser", "getAuthenticatedUserForActions"]
  }
}
```

```typescript
// Before (flag off, or query isn't auth-gated):
export function useEventsGetMyRegistrations(shouldSkip?: boolean) { ... }

// After (flag on, handler calls getAuthenticatedUser):
export function useEventsGetMyRegistrations(shouldSkip: boolean) { ... }
```

**Queries with other args**: TypeScript forbids a required parameter after an
optional one, so if the query also takes optional args, `shouldSkip` (or, for
a paginated query, the `options` object that carries it) is inserted right
after any required args and **before** the optional ones — reordering the
signature relative to the historical trailing position:

```typescript
// getUserSessions(userId?, startDate?, endDate?) with the flag on:
export function useLocationGetUserSessions(
  shouldSkip: boolean,
  userId?: Id<"users"> | null,
  startDate?: string | null,
  endDate?: string | null,
) { ... }
```

This is a deliberate, call-site-breaking change for any already-generated hook
that gains a required (and repositioned) `shouldSkip` when the flag is turned
on — every existing caller needs the new argument added at its new position.
Defaults to `false` for backwards compatibility; other projects using this
same `convex-gen` binary are unaffected unless they opt in.

### API Wrappers (`generators.api: true`)

Type-safe objects mapping function names to API references.

**Features:**

- Organized by function type (queries, mutations, actions)
- Grouped by namespace or split by sub-namespace
- Collision detection for duplicate function names

**Example output:**

```typescript
import type { FunctionReference } from "convex/server";
import { api } from "@organization/backend/api";

export const EventsQueries: Record<string, FunctionReference<"query">> = {
  getEventById: api.events
    .getEventById as unknown as FunctionReference<"query">,
  listEvents: api.events.listEvents as unknown as FunctionReference<"query">,
};

export const EventsMutations: Record<string, FunctionReference<"mutation">> = {
  createEvent: api.events
    .createEvent as unknown as FunctionReference<"mutation">,
};
```

### Types (`generators.types: true`)

TypeScript type definitions derived from Convex schema tables.

**Features:**

- Document types (e.g., `User = Doc<"users">`)
- ID types (e.g., `UserId = Id<"users">`)
- Utility types (table name unions, entity type unions)

**Example output:**

```typescript
import type { Doc, Id } from "@organization/backend/dataModel";

export type { Doc, Id };

// ============================================================================
// TABLE DOCUMENT TYPES
// ============================================================================

/** events table */
export type Events = Doc<"events">;

/** users table */
export type Users = Doc<"users">;

// ============================================================================
// TABLE ID TYPES
// ============================================================================

export type EventsId = Id<"events">;
export type UsersId = Id<"users">;

// ============================================================================
// UTILITY TYPES
// ============================================================================

/** Union of all table names */
export type TableName = "events" | "users";

/** Union of all entity types (singular form) */
export type EntityType = "event" | "user";
```

## Exit Codes

- **`0`** - Success: Code generation completed without errors
- **`1`** - Error: Configuration loading failed, scanning failed, parsing failed, or generation failed

## Command Line Arguments

The tool does not currently accept command-line arguments. All configuration is done via `.convex-gen.json`.

## Environment Variables

No environment variables are required or recognized by convex-gen.

## How It Works

### 1. Configuration Loading

The tool searches for `.convex-gen.json` or `convex-gen.json` in the current directory and applies defaults for missing values.

### 2. Directory Scanning

- **Convex Functions**: Recursively scans the Convex directory for TypeScript files containing `query()`, `mutation()`, or `action()` exports
- **Schema Files**: Scans for `defineSchema()` and `defineTable()` declarations
- Skips directories and files matching configuration patterns

### 3. Function Parsing

For each Convex file:

- Extracts exported function declarations
- Skips internal functions (`internalQuery`, `internalMutation`, `internalAction`)
- Parses function arguments and validators
- Detects pagination support
- Caches validator definitions for reference resolution

### 4. Schema Parsing

For each schema file:

- Extracts table names from `defineSchema()` and `defineTable()` declarations
- Handles both main schema files and individual domain schema files
- Deduplicates table entries

### 5. Code Generation

Generates output files based on configuration:

- Creates hooks matching Convex function signatures
- Creates API reference objects
- Creates TypeScript types for schema tables
- Generates barrel export `index.ts` files

## Example Workflow

```bash
# 1. Create configuration in your project root
cat > .convex-gen.json <<EOF
{
  "org": "@myorg",
  "convex": { "path": "packages/backend" },
  "dataLayer": { "path": "packages/data-layer/src" },
  "generators": {
    "hooks": true,
    "api": true,
    "types": true
  }
}
EOF

# 2. Run convex-gen
convex-gen

# Output:
# convex-gen - Convex Data Layer Generator
#
# Organization: @myorg
# Convex path: packages/backend
# Data layer path: packages/data-layer/src
#
# Building validator cache...
# Cached 12 validators
#
# Scanning Convex functions...
# Found 18 Convex files
# Parsed 45 functions
#
# Scanning schema files...
# Found 1 schema files
# Parsed 8 tables
#
# Generating hooks...
#   18 query hooks
#   15 mutation hooks
#   12 action hooks
#   Output: packages/data-layer/src/generated-hooks
#
# Generating API wrappers...
#   Output: packages/data-layer/src/generated-api
#
# Generating types...
#   8 table types
#   8 ID types
#   Output: packages/data-layer/src/generated-types
#
# Generation complete!

# 3. Use generated code in your application
# Use hooks in React components
# Import API references for type safety
# Use types in function signatures
```

## Troubleshooting

### Config file not found

**Error:** `config file not found (tried: [.convex-gen.json, convex-gen.json])`

**Solution:** Ensure `.convex-gen.json` exists in the current directory.

### Org is required

**Error:** `org is required (e.g., "@dashtag")`

**Solution:** Add an `org` field to your config file.

### Path doesn't exist

**Error:** `convex path does not exist: packages/backend`

**Solution:** Verify the `convex.path` in your config matches your actual directory structure.

### Failed to build validator cache

**Warning:** `failed to build validator cache: [error]`

**Solution:** This is usually non-fatal. Check that validator files exist in the expected locations. The tool will continue with best-effort parsing.

## Notes

- All generated files include a warning not to edit manually
- Generated files are safe to check into version control
- Re-running the tool overwrites previously generated files
- The tool uses regex-based parsing for better TypeScript compatibility
- Validator caching helps resolve complex argument types
- Pagination support is detected via `paginationOptsValidator` references
