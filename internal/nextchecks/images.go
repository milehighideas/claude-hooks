package nextchecks

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CheckImages verifies that every public-relative asset reference found in the
// app's source resolves to a real file under the app's public/ directory.
//
// next build does NOT validate public/ references, so a missing image is a
// silent runtime 404 with a green build — this catches it statically.
func CheckImages(appPath string, cfg ImageConfig) (Result, error) {
	cfg = cfg.WithDefaults()

	publicPath := filepath.Join(appPath, cfg.PublicDir)
	if !dirExists(publicPath) {
		return Result{Skipped: true, Reason: "no " + cfg.PublicDir + "/ directory"}, nil
	}

	re := imageRefRegexp(cfg.Extensions)

	// ref -> first source file it appeared in (deterministic reporting).
	seen := make(map[string]string)
	for _, file := range sourceFiles(appPath, cfg.SrcDirs, cfg.ExcludePaths) {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		for _, m := range re.FindAllStringSubmatch(string(data), -1) {
			ref := stripQueryHash(m[1])
			if imageRefIgnored(ref, cfg.Ignore) {
				continue
			}
			if _, ok := seen[ref]; !ok {
				seen[ref] = file
			}
		}
	}

	res := Result{Scanned: len(seen)}
	for ref, file := range seen {
		target := filepath.Join(publicPath, strings.TrimPrefix(ref, "/"))
		if !fileExists(target) {
			res.Misses = append(res.Misses, Miss{Ref: ref, File: file})
		}
	}
	sortMisses(res.Misses)
	return res, nil
}

// imageRefIgnored reports whether ref matches a configured ignore prefix. Use
// for asset paths Next serves that have no public/ counterpart — e.g. the
// app-router /favicon.ico (from app/favicon.ico).
func imageRefIgnored(ref string, ignore []string) bool {
	for _, ig := range ignore {
		if ig != "" && strings.HasPrefix(ref, ig) {
			return true
		}
	}
	return false
}

// imageRefRegexp builds a matcher for leading-slash asset paths with one of the
// configured extensions. The path must be preceded by a string/attr delimiter
// ("'`, whitespace, =, or `(`) so substrings inside URLs like
// https://cdn/x/y.png (preceded by a letter) are not matched.
func imageRefRegexp(exts []string) *regexp.Regexp {
	quoted := make([]string, len(exts))
	for i, e := range exts {
		quoted[i] = regexp.QuoteMeta(strings.TrimPrefix(e, "."))
	}
	pattern := "(?i)[\"'` (=\\s](/[A-Za-z0-9._\\-/]+\\.(?:" + strings.Join(quoted, "|") + "))"
	return regexp.MustCompile(pattern)
}
