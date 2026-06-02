# validate-next

Standalone validator for Next.js projects. Catches two whole classes of bug
that `next build`, lint, and typecheck all miss:

1. **Missing public assets** — a `/images/foo.png` reference whose file isn't in
   `public/` (silent runtime 404, green build).
2. **Dead internal links** — a `<Link href="/typo">` or `href` pointing at a
   route that doesn't exist.

The detection logic lives in `internal/nextchecks` and is shared with the
`pre-commit` orchestrator (`pre-commit --check nextImageCheck|nextLinkCheck`),
so the two never drift.

## Usage

```bash
validate-next --path . --check both      # images + links (default)
validate-next --path apps/web --check images
validate-next --check links              # static route analysis
```

Reads the `nextImageCheck` / `nextLinkCheck` blocks from the nearest
`.pre-commit.json` (walking up from `--path`).

```jsonc
{
  "apps": { "web": { "path": "apps/web", "filter": "web" } },
  "nextImageCheck": { "apps": ["web"] },
  "nextLinkCheck":  { "apps": ["web"], "mode": "static" }
}
```

- `nextImageCheck`: `srcDirs`, `publicDir`, `extensions`, `excludePaths` (all optional).
- `nextLinkCheck`: `mode` (`static` | `crawl` | `both`), `srcDirs`, `baseUrl` (crawl), `ignore`.
  - **static** (default): builds the route set from `app/` (route groups,
    `[slug]`, `[...catchall]`, `next.config` redirect sources) and validates
    every internal `href` literal. No server needed.
  - **crawl**: fetches `baseUrl` (default `http://localhost:3000`), follows
    `<a href>` links, flags 4xx/5xx (3xx redirects are OK). Requires a running server.
  - **both**: runs static then crawl.

## Exit codes

- `0` — clean
- `1` — error (no config found, bad `--check`)
- `2` — unresolved references found
