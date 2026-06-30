package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/milehighideas/claude-hooks/internal/nextchecks"
)

// NextImageCheckConfig is the .pre-commit.json "nextImageCheck" block. Apps
// scopes which configured apps to check (empty = all); the embedded
// nextchecks.ImageConfig fields (srcDirs/publicDir/extensions/excludePaths)
// are promoted into the same JSON object.
type NextImageCheckConfig struct {
	Apps []string `json:"apps"`
	nextchecks.ImageConfig
}

// NextLinkCheckConfig is the .pre-commit.json "nextLinkCheck" block. The
// embedded nextchecks.LinkConfig contributes mode/srcDirs/baseUrl/ignore.
type NextLinkCheckConfig struct {
	Apps []string `json:"apps"`
	nextchecks.LinkConfig
}

// runNextImageCheck validates public-asset references for the in-scope apps.
func runNextImageCheck(cfg NextImageCheckConfig, apps map[string]AppConfig) error {
	return runNextCheck("Next image refs", "next-image", "image reference", cfg.Apps, apps,
		func(appPath string) (nextchecks.Result, error) {
			return nextchecks.CheckImages(appPath, cfg.WithDefaults())
		})
}

// runNextLinkCheck validates internal links for the in-scope apps.
func runNextLinkCheck(cfg NextLinkCheckConfig, apps map[string]AppConfig) error {
	return runNextCheck("Next link check", "next-link", "link", cfg.Apps, apps,
		func(appPath string) (nextchecks.Result, error) {
			return nextchecks.CheckLinks(appPath, cfg.WithDefaults())
		})
}

// runNextCheck runs a per-app nextchecks function across the scoped apps,
// aggregates misses, and reports in compact or verbose mode like other checks.
func runNextCheck(
	display, subdir, noun string,
	scope []string,
	apps map[string]AppConfig,
	fn func(appPath string) (nextchecks.Result, error),
) error {
	type appRun struct{ name, path string }
	var runs []appRun
	if len(scope) == 0 {
		for name, cfg := range apps {
			runs = append(runs, appRun{name, cfg.Path})
		}
	} else {
		for _, name := range scope {
			if cfg, ok := apps[name]; ok {
				runs = append(runs, appRun{name, cfg.Path})
			}
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].name < runs[j].name })

	if !compactMode() {
		fmt.Println("================================")
		fmt.Printf("  %s\n", display)
		fmt.Println("================================")
	}

	type appMiss struct {
		app string
		nextchecks.Miss
	}
	var misses []appMiss
	var runErr error
	for _, r := range runs {
		res, err := fn(r.path)
		if err != nil {
			runErr = err
			if !compactMode() {
				fmt.Printf("   ⚠️  %s: %v\n", r.name, err)
			}
			continue
		}
		if res.Skipped {
			if !compactMode() {
				fmt.Printf("   ⏩ %s skipped (%s)\n", r.name, res.Reason)
			}
			continue
		}
		for _, m := range res.Misses {
			misses = append(misses, appMiss{app: r.name, Miss: m})
		}
		if !compactMode() {
			if len(res.Misses) == 0 {
				fmt.Printf("   ✓ %s — %d %ss OK\n", r.name, res.Scanned, noun)
			} else {
				for _, m := range res.Misses {
					fmt.Printf("   ❌ %s  %s  (in %s)\n", r.name, m.Ref, m.File)
				}
			}
		}
	}

	failed := len(misses) > 0

	// Render the misses (and any run error) into a report. Written always:
	// findings.txt on failure, fullreport.txt every run.
	var out strings.Builder
	for _, m := range misses {
		fmt.Fprintf(&out, "%s  %s  (in %s)\n", m.app, m.Ref, m.File)
	}
	if runErr != nil {
		fmt.Fprintf(&out, "error: %v\n", runErr)
	}
	_ = writeRunReport(subdir, display, out.String(), failed || runErr != nil)

	if compactMode() {
		if failed {
			printStatus(display, false, fmt.Sprintf("%d %ss", len(misses), noun))
			printReportHint(subdir + "/")
			return fmt.Errorf("found %d unresolved %s(s)", len(misses), noun)
		}
		if runErr != nil {
			printStatus(display, false, "error")
			printReportHint(subdir + "/")
			return runErr
		}
		printStatus(display, true, "")
		return nil
	}

	if failed {
		fmt.Printf("\n❌ Found %d unresolved %s(s)\n\n", len(misses), noun)
		return fmt.Errorf("found %d unresolved %s(s)", len(misses), noun)
	}
	if runErr != nil {
		return runErr
	}
	fmt.Printf("✅ All %ss resolve\n\n", noun)
	return nil
}
