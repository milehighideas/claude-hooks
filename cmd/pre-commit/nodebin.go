package main

import (
	"os"
	"path/filepath"
)

// resolveNodeBin walks up from startDir looking for node_modules/.bin/tool and
// returns its absolute path with found=true. This lets the gate run each
// project's *installed* CLI (tsc, tsgo, tsc-files, eslint, oxlint, lint-staged,
// convex, prettier) rather than a global copy or an npx/bunx-fetched one, so the
// gate can never drift from the versions `bun run`/CI use.
//
// A missing or half-written node_modules (fresh clone, an install still in
// flight, or a broken .bin symlink) makes os.Stat fail, so found is false and
// the caller decides how to degrade — never treating a transient state as a
// real result. When not found the returned name is the bare tool, which callers
// must NOT run through a network-fetching runner.
func resolveNodeBin(startDir, tool string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return tool, false
	}
	for {
		candidate := filepath.Join(dir, "node_modules", ".bin", tool)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return tool, false
		}
		dir = parent
	}
}
