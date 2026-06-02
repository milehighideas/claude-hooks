// Package nextchecks implements static (and optional runtime) validation for
// Next.js apps that build- and type-checks miss: references to public/ assets
// that don't exist, and internal <Link>/href targets that resolve to no route.
//
// The logic lives here, free of any package-main coupling, so both the
// pre-commit orchestrator (cmd/pre-commit) and the standalone validator
// (cmd/validate-next) call exactly the same code.
package nextchecks

// Miss is a single failed reference: a public asset path or internal link that
// could not be resolved, plus the source file it was found in.
type Miss struct {
	Ref  string // e.g. "/images/owners.jpg" or "/rooftalk/typo"
	File string // source file the reference was found in (app-relative)
}

// Result is the outcome of one check for one app.
type Result struct {
	Skipped bool   // true when the app isn't a Next.js app (no public/ or app/ dir)
	Reason  string // why it was skipped (for reporting)
	Scanned int    // number of distinct references/links examined
	Misses  []Miss // unresolved references
}

// ImageConfig controls the public-asset reference check.
type ImageConfig struct {
	SrcDirs      []string `json:"srcDirs"`
	PublicDir    string   `json:"publicDir"`
	Extensions   []string `json:"extensions"`
	ExcludePaths []string `json:"excludePaths"`
}

// LinkConfig controls the internal-link check.
type LinkConfig struct {
	Mode    string   `json:"mode"` // "static" (default) | "crawl" | "both"
	SrcDirs []string `json:"srcDirs"`
	BaseURL string   `json:"baseUrl"` // crawl mode target
	Ignore  []string `json:"ignore"`  // link path prefixes to skip
}

var (
	defaultSrcDirs    = []string{"app", "components", "lib", "src"}
	defaultExtensions = []string{
		"jpg", "jpeg", "png", "webp", "svg", "gif", "avif", "ico",
	}
	defaultPublicDir = "public"
	defaultBaseURL   = "http://localhost:3000"
)

// WithDefaults returns a copy with empty fields filled in.
func (c ImageConfig) WithDefaults() ImageConfig {
	if len(c.SrcDirs) == 0 {
		c.SrcDirs = append([]string(nil), defaultSrcDirs...)
	}
	if c.PublicDir == "" {
		c.PublicDir = defaultPublicDir
	}
	if len(c.Extensions) == 0 {
		c.Extensions = append([]string(nil), defaultExtensions...)
	}
	return c
}

// WithDefaults returns a copy with empty fields filled in.
func (c LinkConfig) WithDefaults() LinkConfig {
	if c.Mode == "" {
		c.Mode = "static"
	}
	if len(c.SrcDirs) == 0 {
		c.SrcDirs = append([]string(nil), defaultSrcDirs...)
	}
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	return c
}
