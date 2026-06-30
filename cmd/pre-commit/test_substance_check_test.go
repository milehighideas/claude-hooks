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
	// One subfolder per app/package, each holding findings.txt + fullreport.txt.
	for _, want := range []string{"mobile", "story", "backend"} {
		if !got[want] {
			t.Errorf("expected per-app report folder %s, missing; got: %v", want, got)
		}
		for _, file := range []string{"findings.txt", "fullreport.txt"} {
			if _, err := os.Stat(filepath.Join(outDir, want, file)); err != nil {
				t.Errorf("expected %s/%s: %v", want, file, err)
			}
		}
	}
	if got["report.txt"] || got["story.txt"] {
		t.Errorf("found legacy flat report file; expected per-app subfolders only: %v", got)
	}
	if len(entries) != 3 {
		t.Errorf("expected exactly 3 per-app folders, got %d (%v)", len(entries), got)
	}

	// story/fullreport.txt aggregates both story files and surfaces their kinds.
	storyBody := readFile(t, filepath.Join(outDir, "story", "fullreport.txt"))
	if !strings.Contains(storyBody, "Files with violations: 2") {
		t.Errorf("story fullreport should report 2 files; got:\n%s", storyBody)
	}
	for _, want := range []string{"TEST SUBSTANCE - STORY", "[loc_ratio_below]", "tautological_assertions] 3", "B.tsx", "C.tsx"} {
		if !strings.Contains(storyBody, want) {
			t.Errorf("story fullreport missing %q; got:\n%s", want, storyBody)
		}
	}

	// story/findings.txt lists the violating sources concisely (no raw detail).
	storyFindings := readFile(t, filepath.Join(outDir, "story", "findings.txt"))
	for _, want := range []string{"FINDINGS: story", "Blocking findings: 2", "B.tsx", "C.tsx"} {
		if !strings.Contains(storyFindings, want) {
			t.Errorf("story findings missing %q; got:\n%s", want, storyFindings)
		}
	}

	// mobile/fullreport.txt carries the majority_weak line.
	mobileBody := readFile(t, filepath.Join(outDir, "mobile", "fullreport.txt"))
	if !strings.Contains(mobileBody, "[majority_weak]") {
		t.Errorf("mobile fullreport missing majority_weak line; got:\n%s", mobileBody)
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
