package nextchecks

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sortMisses orders misses deterministically (by ref, then file) for stable output.
func sortMisses(m []Miss) {
	sort.Slice(m, func(i, j int) bool {
		if m[i].Ref != m[j].Ref {
			return m[i].Ref < m[j].Ref
		}
		return m[i].File < m[j].File
	})
}

var sourceExts = map[string]bool{
	".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".mdx": true,
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sourceFiles returns every source file under the given srcDirs (relative to
// appPath), skipping node_modules/.next/.turbo and any path containing one of
// excludePaths.
func sourceFiles(appPath string, srcDirs, excludePaths []string) []string {
	var out []string
	for _, dir := range srcDirs {
		root := filepath.Join(appPath, dir)
		if !dirExists(root) {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == "node_modules" || name == ".next" || name == ".turbo" || name == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if !sourceExts[strings.ToLower(filepath.Ext(path))] {
				return nil
			}
			for _, ex := range excludePaths {
				if ex != "" && strings.Contains(path, ex) {
					return nil
				}
			}
			out = append(out, path)
			return nil
		})
	}
	return out
}

// stripQueryHash removes ?query and #hash suffixes from a path/link.
func stripQueryHash(ref string) string {
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		return ref[:i]
	}
	return ref
}
