package main

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setupProject writes a minimal monorepo with one Next app under apps/web and
// returns the project root.
func setupProject(t *testing.T, pageBody string) string {
	root := t.TempDir()
	write(t, filepath.Join(root, ".pre-commit.json"), `{
		"apps": { "web": { "path": "apps/web", "filter": "web" } },
		"nextImageCheck": { "apps": ["web"] },
		"nextLinkCheck": { "apps": ["web"], "mode": "static" }
	}`)
	write(t, filepath.Join(root, "apps", "web", "public", "a.png"), "x")
	write(t, filepath.Join(root, "apps", "web", "app", "page.tsx"), "export default function P(){return null}")
	write(t, filepath.Join(root, "apps", "web", "app", "about", "page.tsx"), "export default function P(){return null}")
	write(t, filepath.Join(root, "apps", "web", "components", "nav.tsx"), pageBody)
	return root
}

func TestRunClean(t *testing.T) {
	root := setupProject(t, `import Link from "next/link";
		export function Nav(){return (<><img src="/a.png"/><Link href="/about">a</Link></>)}`)
	pathFlag, checkFlag, helpFlag = root, "both", false
	if rc := run(); rc != 0 {
		t.Fatalf("want exit 0 (clean), got %d", rc)
	}
}

func TestRunDetectsMissing(t *testing.T) {
	root := setupProject(t, `import Link from "next/link";
		export function Nav(){return (<><img src="/missing.png"/><Link href="/nope">x</Link></>)}`)

	pathFlag, checkFlag, helpFlag = root, "images", false
	if rc := run(); rc != 2 {
		t.Fatalf("images: want exit 2 (missing asset), got %d", rc)
	}
	pathFlag, checkFlag, helpFlag = root, "links", false
	if rc := run(); rc != 2 {
		t.Fatalf("links: want exit 2 (dead link), got %d", rc)
	}
}

func TestLoadConfigWalksUp(t *testing.T) {
	root := setupProject(t, `<div/>`)
	dir, cfg, err := loadConfig(filepath.Join(root, "apps", "web", "app"))
	if err != nil {
		t.Fatal(err)
	}
	if dir != root {
		t.Fatalf("config dir = %q, want %q", dir, root)
	}
	if _, ok := cfg.Apps["web"]; !ok {
		t.Fatal("expected app 'web' in parsed config")
	}
}
