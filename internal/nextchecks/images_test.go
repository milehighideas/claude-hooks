package nextchecks

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckImages(t *testing.T) {
	app := t.TempDir()
	writeFile(t, filepath.Join(app, "public", "a.png"), "x")
	writeFile(t, filepath.Join(app, "public", "images", "hero.jpg"), "x")
	writeFile(t, filepath.Join(app, "app", "page.tsx"), `
		import Image from "next/image";
		export default function P() {
			return (<>
				<Image src="/a.png" />
				<Image src="/images/hero.jpg" />
				<Image src="/images/missing.png" />
				<a href="https://cdn.example.com/remote/photo.png">x</a>
			</>);
		}
	`)

	res, err := CheckImages(app, ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped {
		t.Fatal("should not skip — public/ exists")
	}
	if len(res.Misses) != 1 {
		t.Fatalf("want 1 miss, got %d: %+v", len(res.Misses), res.Misses)
	}
	if res.Misses[0].Ref != "/images/missing.png" {
		t.Fatalf("want /images/missing.png, got %q", res.Misses[0].Ref)
	}
	// The remote https URL must NOT be treated as a public asset.
	for _, m := range res.Misses {
		if m.Ref == "/remote/photo.png" {
			t.Fatal("remote URL path was wrongly treated as a public asset")
		}
	}
}

func TestCheckImagesSkipsNonNext(t *testing.T) {
	app := t.TempDir()
	writeFile(t, filepath.Join(app, "app", "page.tsx"), `<img src="/x.png"/>`)
	res, err := CheckImages(app, ImageConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Fatal("want skipped (no public/ dir)")
	}
}
