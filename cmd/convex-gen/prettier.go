package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// formatTSWithPrettier runs the project's Prettier (in --write mode) over the
// given generated TypeScript files so the emitter's raw, single-line output is
// reformatted to the committed, human-reviewed style.
//
// Why this lives in the generator: the curated public-API TS files (the
// terraform <res>Api.ts / <res>Routes.ts / types) are NOT in the project's
// .prettierignore — they are committed Prettier-formatted. The emitter writes
// dense, unformatted TS, so without this pass the generated files drift from the
// committed copies every time convex-gen runs outside an editor/pre-commit that
// formats afterwards (e.g. a CI deploy or the file watcher). Formatting here
// makes convex-gen's output self-consistent: running it reproduces the committed
// bytes regardless of who invokes it.
//
// Prettier is invoked from the current working directory (the project root, where
// convex-gen runs), so it picks up the project's prettier config AND its
// .prettierignore — which correctly skips the data-layer generated files that are
// intentionally committed raw. Best-effort: if no Prettier is resolvable the step
// is skipped with a warning rather than failing generation, keeping convex-gen
// usable in projects that don't depend on Prettier.
func formatTSWithPrettier(files []string) error {
	// Keep only files that exist on disk (a generator may skip some outputs).
	present := make([]string, 0, len(files))
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			present = append(present, f)
		}
	}
	if len(present) == 0 {
		return nil
	}

	bin, prefix := resolvePrettier()
	if bin == "" {
		fmt.Println("  (prettier not found on PATH or in node_modules — skipping format of generated TS)")
		return nil
	}

	// --log-level warn silences the per-file "formatted" lines; --write edits in
	// place. Prettier still honors .prettierignore for explicitly-passed paths.
	args := append(append([]string{}, prefix...), "--log-level", "warn", "--write")
	args = append(args, present...)

	cmd := exec.Command(bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("  Warning: prettier failed to format generated TS (%v); leaving emitter output as-is.\n", err)
		if s := stderr.String(); s != "" {
			fmt.Printf("    %s\n", s)
		}
		return nil
	}
	fmt.Printf("  Formatted %d generated file(s) with prettier\n", len(present))
	return nil
}

// resolvePrettier locates a Prettier executable, preferring the project-local
// install so the project's pinned version + config are used. Returns the binary
// path and any leading args needed to invoke prettier through it (e.g. the
// package name for bunx/npx). Returns ("", nil) when none is found.
func resolvePrettier() (string, []string) {
	// Project-local install (cwd is the project root when convex-gen runs).
	local := filepath.Join("node_modules", ".bin", "prettier")
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local, nil
	}
	if p, err := exec.LookPath("prettier"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("bunx"); err == nil {
		return p, []string{"prettier"}
	}
	if p, err := exec.LookPath("npx"); err == nil {
		return p, []string{"--no-install", "prettier"}
	}
	return "", nil
}
