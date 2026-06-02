package nextchecks

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CheckLinks validates internal links. Mode selects the strategy:
//   - "static" (default): build the route set from the app/ directory and
//     verify every internal href literal resolves to a route. No server needed.
//   - "crawl": fetch the running site (BaseURL), follow <a href> links, flag 4xx/5xx.
//   - "both": run static then crawl, aggregating misses.
func CheckLinks(appPath string, cfg LinkConfig) (Result, error) {
	cfg = cfg.WithDefaults()

	switch cfg.Mode {
	case "crawl":
		return crawlLinks(cfg)
	case "both":
		staticRes, err := checkLinksStatic(appPath, cfg)
		if err != nil {
			return staticRes, err
		}
		crawlRes, err := crawlLinks(cfg)
		staticRes.Scanned += crawlRes.Scanned
		staticRes.Misses = append(staticRes.Misses, crawlRes.Misses...)
		sortMisses(staticRes.Misses)
		return staticRes, err
	default: // "static"
		return checkLinksStatic(appPath, cfg)
	}
}

// ---------------------------------------------------------------------------
// Static route analysis
// ---------------------------------------------------------------------------

type segKind int

const (
	segLiteral segKind = iota
	segDynamic
	segCatchAll
	segOptCatchAll
)

type routeSeg struct {
	kind  segKind
	value string
}

type route []routeSeg

type link struct {
	path   string
	file   string
	prefix bool // true = captured from a truncated template literal (`/a/${x}`)
}

func checkLinksStatic(appPath string, cfg LinkConfig) (Result, error) {
	appDir := filepath.Join(appPath, "app")
	if !dirExists(appDir) {
		return Result{Skipped: true, Reason: "no app/ directory (App Router only)"}, nil
	}

	routes := buildRouteSet(appDir)
	routes = append(routes, parseRedirectSources(appPath)...)

	links := collectLinks(appPath, cfg)

	res := Result{Scanned: len(links)}
	for _, l := range links {
		if linkResolves(l, routes) {
			continue
		}
		res.Misses = append(res.Misses, Miss{Ref: l.path, File: l.file})
	}
	sortMisses(res.Misses)
	return res, nil
}

// buildRouteSet walks the App Router tree and returns a matcher per routable
// directory (one containing a page.* or route.* file).
func buildRouteSet(appDir string) []route {
	var routes []route
	pageFile := regexp.MustCompile(`^(page|route)\.(tsx|jsx|ts|js|mjs)$`)

	_ = filepath.WalkDir(appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !pageFile.MatchString(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(appDir, filepath.Dir(path))
		if err != nil {
			return nil
		}
		r, ok := routeFromRel(rel)
		if ok {
			routes = append(routes, r)
		}
		return nil
	})
	return routes
}

// routeFromRel turns a directory path relative to app/ into a route matcher.
// Returns ok=false for non-routable trees (private _folders).
func routeFromRel(rel string) (route, bool) {
	if rel == "." {
		return route{}, true // app/page.tsx -> "/"
	}
	var r route
	for _, raw := range strings.Split(rel, string(filepath.Separator)) {
		switch {
		case raw == "":
			continue
		case strings.HasPrefix(raw, "_"):
			return nil, false // private folder — not routable
		case strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")"):
			continue // route group — no URL segment
		case strings.HasPrefix(raw, "@"):
			continue // parallel route slot — no URL segment
		case strings.HasPrefix(raw, "[[...") && strings.HasSuffix(raw, "]]"):
			r = append(r, routeSeg{kind: segOptCatchAll})
		case strings.HasPrefix(raw, "[...") && strings.HasSuffix(raw, "]"):
			r = append(r, routeSeg{kind: segCatchAll})
		case strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]"):
			r = append(r, routeSeg{kind: segDynamic})
		default:
			r = append(r, routeSeg{kind: segLiteral, value: raw})
		}
	}
	return r, true
}

// parseRedirectSources best-effort extracts `source: "/..."` paths from a
// next.config.{mjs,js,ts} redirects()/rewrites() block so links pointing at a
// redirect source aren't flagged as dead.
func parseRedirectSources(appPath string) []route {
	re := regexp.MustCompile(`source:\s*["'` + "`" + `](/[^"'` + "`" + `]*)["'` + "`" + `]`)
	var routes []route
	for _, name := range []string{"next.config.mjs", "next.config.js", "next.config.ts"} {
		data, err := os.ReadFile(filepath.Join(appPath, name))
		if err != nil {
			continue
		}
		for _, m := range re.FindAllStringSubmatch(string(data), -1) {
			src := stripQueryHash(m[1])
			// Ignore next.config path-matching syntax (:param, :path*, regex).
			if strings.ContainsAny(src, ":*(") {
				continue
			}
			if r, ok := routeFromRel(toRel(src)); ok {
				routes = append(routes, r)
			}
		}
	}
	return routes
}

func toRel(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return "."
	}
	return p
}

// linkResolves reports whether a collected link matches any route.
func linkResolves(l link, routes []route) bool {
	segs := splitPath(l.path)
	if l.prefix {
		return prefixMatches(segs, routes)
	}
	for _, r := range routes {
		if matchRoute(segs, r) {
			return true
		}
	}
	return false
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchRoute(link []string, r route) bool {
	li, ri := 0, 0
	for ri < len(r) {
		seg := r[ri]
		switch seg.kind {
		case segCatchAll:
			return ri == len(r)-1 && (len(link)-li) >= 1
		case segOptCatchAll:
			return ri == len(r)-1
		default:
			if li >= len(link) {
				return false
			}
			if seg.kind == segLiteral && link[li] != seg.value {
				return false
			}
			li++
			ri++
		}
	}
	return li == len(link)
}

// prefixMatches handles truncated template links (`/a/${x}`): the literal
// prefix segments must match the leading segments of some route.
func prefixMatches(prefix []string, routes []route) bool {
	if len(prefix) == 0 {
		return true // e.g. `/${x}` — unverifiable, don't flag
	}
	for _, r := range routes {
		if len(r) < len(prefix) {
			continue
		}
		ok := true
		for i, ps := range prefix {
			if r[i].kind == segLiteral && r[i].value != ps {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Link collection
// ---------------------------------------------------------------------------

var (
	reHrefStr    = regexp.MustCompile(`href=(?:"|\{")(/[^"]*)"`)
	reHrefSingle = regexp.MustCompile(`href=\{'(/[^']*)'`)
	reHrefTmpl   = regexp.MustCompile("href=\\{`(/[^`$]*)")
)

func collectLinks(appPath string, cfg LinkConfig) []link {
	type key struct {
		path   string
		prefix bool
	}
	seen := make(map[key]string)
	add := func(raw, file string, prefix bool) {
		p := stripQueryHash(raw)
		if !shouldCheckLink(p, cfg.Ignore) {
			return
		}
		k := key{p, prefix}
		if _, ok := seen[k]; !ok {
			seen[k] = file
		}
	}

	for _, file := range sourceFiles(appPath, cfg.SrcDirs, nil) {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		text := string(data)
		for _, m := range reHrefStr.FindAllStringSubmatch(text, -1) {
			add(m[1], file, false)
		}
		for _, m := range reHrefSingle.FindAllStringSubmatch(text, -1) {
			add(m[1], file, false)
		}
		for _, m := range reHrefTmpl.FindAllStringSubmatch(text, -1) {
			add(m[1], file, true)
		}
	}

	out := make([]link, 0, len(seen))
	for k, file := range seen {
		out = append(out, link{path: k.path, file: file, prefix: k.prefix})
	}
	return out
}

// shouldCheckLink filters out links that aren't internal page routes.
func shouldCheckLink(p string, ignore []string) bool {
	if p == "" || strings.HasPrefix(p, "//") {
		return false // empty or protocol-relative/external
	}
	if strings.HasPrefix(p, "/_next") || strings.HasPrefix(p, "/api") {
		return false
	}
	for _, ig := range ignore {
		if ig != "" && strings.HasPrefix(p, ig) {
			return false
		}
	}
	// A trailing segment with a file extension (e.g. /sitemap.xml, /resume.pdf,
	// /favicon.ico) is a static file, not a page route.
	last := p
	if i := strings.LastIndex(p, "/"); i >= 0 {
		last = p[i+1:]
	}
	if strings.Contains(last, ".") {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Runtime crawl
// ---------------------------------------------------------------------------

func crawlLinks(cfg LinkConfig) (Result, error) {
	base := strings.TrimRight(cfg.BaseURL, "/")
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // treat 3xx as terminal-OK
		},
	}

	// Reachability probe.
	if _, err := client.Get(base + "/"); err != nil {
		return Result{}, fmt.Errorf("crawl: could not reach %s (start the server first): %w", base, err)
	}

	seeds := map[string]string{"/": "(seed)"}
	for _, p := range sitemapPaths(client, base) {
		if _, ok := seeds[p]; !ok {
			seeds[p] = "(sitemap)"
		}
	}

	status := make(map[string]int)
	foundOn := make(map[string]string)
	queue := make([]string, 0, len(seeds))
	for p, ref := range seeds {
		queue = append(queue, p)
		foundOn[p] = ref
	}

	res := Result{}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if _, done := status[p]; done {
			continue
		}
		code, body := fetch(client, base+p)
		status[p] = code
		res.Scanned++
		if code < 0 || code >= 400 {
			res.Misses = append(res.Misses, Miss{Ref: p, File: "crawl: linked from " + foundOn[p]})
			continue
		}
		for _, m := range reHrefStr.FindAllStringSubmatch(body, -1) {
			next := stripQueryHash(m[1])
			if !shouldCheckLink(next, cfg.Ignore) {
				continue
			}
			if _, done := status[next]; !done {
				if _, q := foundOn[next]; !q {
					foundOn[next] = p
				}
				queue = append(queue, next)
			}
		}
	}
	sortMisses(res.Misses)
	return res, nil
}

func fetch(client *http.Client, u string) (int, string) {
	resp, err := client.Get(u)
	if err != nil {
		return -1, ""
	}
	defer func() { _ = resp.Body.Close() }()
	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.Contains(ct, "text/html") {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
		return resp.StatusCode, string(b)
	}
	return resp.StatusCode, ""
}

func sitemapPaths(client *http.Client, base string) []string {
	resp, err := client.Get(base + "/sitemap.xml")
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	re := regexp.MustCompile(`<loc>([^<]+)</loc>`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		if u, err := url.Parse(strings.TrimSpace(m[1])); err == nil && u.Path != "" {
			out = append(out, u.Path)
		}
	}
	return out
}
