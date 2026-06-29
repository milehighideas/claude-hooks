package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatTSWithPrettier_NoFilesIsNoOp(t *testing.T) {
	if err := formatTSWithPrettier(nil); err != nil {
		t.Fatalf("nil input should be a no-op, got %v", err)
	}
	if err := formatTSWithPrettier([]string{}); err != nil {
		t.Fatalf("empty input should be a no-op, got %v", err)
	}
}

func TestFormatTSWithPrettier_SkipsNonexistentFiles(t *testing.T) {
	// All paths missing → nothing to format → no-op, no prettier invocation,
	// no error (the generator must not fail just because an output was skipped).
	missing := filepath.Join(t.TempDir(), "does-not-exist.ts")
	if err := formatTSWithPrettier([]string{missing}); err != nil {
		t.Fatalf("nonexistent files should be filtered to a no-op, got %v", err)
	}
}

func TestResolvePrettier_PrefersProjectLocal(t *testing.T) {
	// Run from a temp dir containing a fake node_modules/.bin/prettier so the
	// resolver picks the project-local install ahead of anything on PATH.
	dir := t.TempDir()
	binDir := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(binDir, "prettier")
	if err := os.WriteFile(local, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	bin, prefix := resolvePrettier()
	if bin != filepath.Join("node_modules", ".bin", "prettier") {
		t.Fatalf("expected project-local prettier, got %q", bin)
	}
	if len(prefix) != 0 {
		t.Fatalf("project-local prettier needs no prefix args, got %v", prefix)
	}
}
