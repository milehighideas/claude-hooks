package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/milehighideas/claude-hooks/internal/substance"
)

// sampleSubstanceReport builds a report spanning two apps and one package so
// the per-app split has something to fan out across. Source paths are rooted
// under projectRoot so writeSubstanceReport can relativize them.
func sampleSubstanceReport(projectRoot string) *substanceReport {
	return &substanceReport{Files: []substanceFileResult{
		{
			Source: filepath.Join(projectRoot, "apps/mobile/components/A.tsx"),
			Test:   filepath.Join(projectRoot, "apps/mobile/components/A.test.tsx"),
			Substance: []substance.Violation{
				{Kind: "no_interaction_in_ui_test", Message: "render-only"},
			},
			MajorityWeak: true,
		},
		{
			Source: filepath.Join(projectRoot, "apps/story/components/B.tsx"),
			Test:   filepath.Join(projectRoot, "apps/story/components/B.test.tsx"),
			Substance: []substance.Violation{
				{Kind: "loc_ratio_below", Message: "too thin"},
			},
		},
		{
			Source: filepath.Join(projectRoot, "apps/story/components/C.tsx"),
			Test:   filepath.Join(projectRoot, "apps/story/components/C.test.tsx"),
			Tautologies: 3,
		},
		{
			Source: filepath.Join(projectRoot, "packages/backend/x.ts"),
			Test:   filepath.Join(projectRoot, "packages/backend/x.test.ts"),
			Substance: []substance.Violation{
				{Kind: "branch_imbalance", Message: "needs more it()"},
			},
		},
	}}
}

func TestWriteSubstanceReport_PerAppSplit(t *testing.T) {
	root := t.TempDir()
	baseDir := t.TempDir()
	rep := sampleSubstanceReport(root)

	if err := writeSubstanceReport(rep, root, baseDir); err != nil {
		t.Fatalf("writeSubstanceReport: %v", err)
	}

	outDir := filepath.Join(baseDir, "test-substance")
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}

	got := make(map[string]bool)
	for _, e := range entries {
		got[e.Name()] = true
	}
	// One file per app/package, NOT a single report.txt.
	for _, want := range []string{"mobile.txt", "story.txt", "backend.txt"} {
		if !got[want] {
			t.Errorf("expected per-app report %s, missing; got files: %v", want, got)
		}
	}
	if got["report.txt"] {
		t.Errorf("found legacy single report.txt; expected per-app files only")
	}
	if len(entries) != 3 {
		t.Errorf("expected exactly 3 per-app files, got %d (%v)", len(entries), got)
	}

	// story.txt aggregates both story files and surfaces their kinds.
	storyBody := readFile(t, filepath.Join(outDir, "story.txt"))
	if !strings.Contains(storyBody, "Files with violations: 2") {
		t.Errorf("story.txt should report 2 files; got:\n%s", storyBody)
	}
	for _, want := range []string{"TEST SUBSTANCE - STORY", "[loc_ratio_below]", "tautological_assertions] 3", "B.tsx", "C.tsx"} {
		if !strings.Contains(storyBody, want) {
			t.Errorf("story.txt missing %q; got:\n%s", want, storyBody)
		}
	}

	// mobile.txt carries the majority_weak line.
	mobileBody := readFile(t, filepath.Join(outDir, "mobile.txt"))
	if !strings.Contains(mobileBody, "[majority_weak]") {
		t.Errorf("mobile.txt missing majority_weak line; got:\n%s", mobileBody)
	}
}

func TestWriteSubstanceReport_NoFilesNoOp(t *testing.T) {
	baseDir := t.TempDir()
	if err := writeSubstanceReport(&substanceReport{}, t.TempDir(), baseDir); err != nil {
		t.Fatalf("writeSubstanceReport empty: %v", err)
	}
	// An empty report writes nothing — not even the test-substance dir.
	if _, err := os.Stat(filepath.Join(baseDir, "test-substance")); !os.IsNotExist(err) {
		t.Errorf("expected no test-substance dir for empty report, stat err=%v", err)
	}
}

func TestSubstanceAppBreakdownDetail_GroupsAndSorts(t *testing.T) {
	root := "/repo"
	rep := sampleSubstanceReport(root)
	detail := substanceAppBreakdownDetail(rep, root)
	want := "backend 1 file(s), mobile 1 file(s), story 2 file(s)"
	if detail != want {
		t.Errorf("breakdown = %q, want %q", detail, want)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
