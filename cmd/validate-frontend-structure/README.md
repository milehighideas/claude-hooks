# validate-frontend-structure

A Claude Code PreToolUse hook that validates frontend folder structure compliance.

## Overview

This hook ensures all route features in a frontend codebase follow a consistent structure with:

- All CRUD folders (create, read, update, delete)
- Additional folders: hooks/, screens/, types/, utils/
- Barrel exports (index.ts) in each folder
- .gitkeep files in all folders
- No loose .tsx files in components/ root

## Exit Codes

- **0**: Allow operation (validation passed or not applicable)
- **2**: Block operation (validation failed)

## Configuration

The hook only runs when explicitly enabled via environment variable:

```bash
export CLAUDE_HOOKS_AST_VALIDATION=true
```

You can set this in `.claude-hooks-config.sh` in your project root.

## Validated Operations

Only checks structure-modifying operations in `/components/` directories:

- **Write** operations creating/modifying `.ts` or `.tsx` files
- **Edit** operations on `.ts` or `.tsx` files
- **Bash** commands using `mkdir`, `touch`, `mv`, or `cp`

Delete operations (`rm`) are not validated.

## Supported Directory Structures

The hook validates both:

- `components/` in project root
- `apps/web/components/` in monorepo structure

## Required Structure

Each feature folder in `components/routes/` and `components/shared/` must have:

```bash
feature-name/
├── index.ts              # Main barrel export
├── create/
│   ├── index.ts
│   └── .gitkeep
├── read/
│   ├── index.ts
│   └── .gitkeep
├── update/
│   ├── index.ts
│   └── .gitkeep
├── delete/
│   ├── index.ts
│   └── .gitkeep
├── hooks/
│   ├── index.ts
│   └── .gitkeep
├── screens/
│   ├── index.ts
│   └── .gitkeep
├── types/
│   ├── index.ts
│   └── .gitkeep
└── utils/
    ├── index.ts
    └── .gitkeep
```

## Building

```bash
go build -o validate-frontend-structure
```

## Testing

```bash
go test -v
```

## Installation

Copy the binary to your Claude hooks directory:

```bash
cp validate-frontend-structure ~/.claude/hooks/
```

## Original Implementation

Converted from Python implementation at:
`~/.claude/hooks/validate-frontend-structure.py`
