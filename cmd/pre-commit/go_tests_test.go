package main

import (
	"os"
	"path/filepath"
	"testing"
)

// newGoModule writes a minimal module at root/sub with the given test body.
func newGoModule(t *testing.T, dir, testBody string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "go.mod"), "module tmp/mod\n\ngo 1.21\n")
	writeFile(t, filepath.Join(dir, "mod.go"), "package mod\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, filepath.Join(dir, "mod_test.go"), testBody)
}

func TestRunGoTestsPasses(t *testing.T) {
	root := t.TempDir()
	mod := filepath.Join(root, "svc")
	newGoModule(t, mod, "package mod\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T){ if Add(1,2)!=3 {t.Fatal(\"bad\")} }\n")

	cfg := GoTestsConfig{Modules: []string{"svc"}}
	staged := []string{filepath.Join(mod, "mod.go")}
	if err := runGoTests(cfg, root, staged); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestRunGoTestsFails(t *testing.T) {
	root := t.TempDir()
	mod := filepath.Join(root, "svc")
	newGoModule(t, mod, "package mod\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T){ if Add(1,2)!=4 {t.Fatal(\"boom\")} }\n")

	cfg := GoTestsConfig{Modules: []string{"svc"}}
	staged := []string{filepath.Join(mod, "mod.go")}
	if err := runGoTests(cfg, root, staged); err == nil {
		t.Fatal("expected failing go test to block")
	}
}

func TestRunGoTestsNoGoFilesIsNoop(t *testing.T) {
	root := t.TempDir()
	cfg := GoTestsConfig{Modules: []string{"svc"}}
	if err := runGoTests(cfg, root, []string{filepath.Join(root, "web", "app.ts")}); err != nil {
		t.Fatalf("no staged go files should be a no-op, got %v", err)
	}
}

func TestGoTestTargetsWholeModuleVsAffected(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "svc", "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	goFiles := []string{filepath.Join(root, "svc", "pkg", "x.go")}

	whole := goTestTargets(GoTestsConfig{Modules: []string{"svc"}}, root, goFiles)
	if len(whole) != 1 || whole[0].pattern != "./..." || filepath.Base(whole[0].dir) != "svc" {
		t.Fatalf("whole-module targets = %+v", whole)
	}

	aff := goTestTargets(GoTestsConfig{Modules: []string{"svc"}, AffectedOnly: true}, root, goFiles)
	if len(aff) != 1 || aff[0].pattern != "." || filepath.Base(aff[0].dir) != "pkg" {
		t.Fatalf("affected targets = %+v", aff)
	}
}
