# validate-next

Validates Next.js projects for two whole classes of bug that `next build`,
lint, and typecheck all pass over:

1. **Missing `public/` assets** — e.g. `<Image src="/images/owners.jpg" />`
   whose file was never added to `public/`. Next does not validate `public/`
   references at build time, so it's a silent runtime 404 with a green build.
2. **Dead internal links** — `<Link href="/typo">` / `href="/old-path"`
   pointing at a route that doesn't exist.

Available two ways, sharing one implementation (`internal/nextchecks`):

- **Standalone binary:** `validate-next --path . --check both`
- **Pre-commit check:** `pre-commit --check nextImageCheck` / `nextLinkCheck`,
  or via `features.nextImageCheck` / `features.nextLinkCheck` in `.pre-commit.json`.

## Configuration (`.pre-commit.json`)

```jsonc
{
  "features": { "nextImageCheck": true, "nextLinkCheck": true },
  "nextImageCheck": { "apps": ["web"] },
  "nextLinkCheck":  { "apps": ["web"], "mode": "static" }
}
```

Each block's `apps` lists which configured apps to check (empty = all). A
non-Next app (no `public/` or `app/` directory) is skipped gracefully.

### `nextImageCheck`

| Field          | Default                                  | Purpose |
| -------------- | ---------------------------------------- | ------- |
| `srcDirs`      | `["app","components","lib","src"]`       | Dirs (relative to the app) scanned for references |
| `publicDir`    | `"public"`                               | Where assets must resolve |
| `extensions`   | `jpg jpeg png webp svg gif avif ico`     | Asset extensions treated as references |
| `excludePaths` | `[]`                                     | Substrings; matching source files are skipped |

### `nextLinkCheck`

| Field     | Default                   | Purpose |
| --------- | ------------------------- | ------- |
| `mode`    | `"static"`                | `static` \| `crawl` \| `both` |
| `srcDirs` | `["app","components","lib","src"]` | Dirs scanned for `href` literals |
| `baseUrl` | `http://localhost:3000`   | Target for crawl mode |
| `ignore`  | `[]`                      | Link path prefixes to skip |

- **static** (default, pre-commit safe): builds the route set from the App
  Router `app/` tree — handling route groups `(group)`, dynamic `[slug]`,
  catch-all `[...slug]`/`[[...slug]]`, and `next.config` redirect `source:`
  paths — then verifies every internal `href` literal resolves. Template-literal
  links (`` href={`/blog/${slug}`} ``) are matched by their static prefix
  against dynamic routes. No server required.
- **crawl**: fetches `baseUrl`, seeds from `/sitemap.xml`, follows `<a href>`
  links breadth-first, and flags any 4xx/5xx (3xx redirects are OK). Requires a
  running server.
- **both**: runs static then crawl.

## Exit codes

- `0` — clean
- `1` — error (no config / bad `--check`)
- `2` — unresolved references found
