package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/milehighideas/claude-hooks/internal/srpnative"
)

// SRPNativeConfig configures native (Swift/Kotlin) Single Responsibility
// checking. It is the native sibling of SRPConfig: same staged-file scoping and
// per-app reporting, but a native-appropriate rule set with line-count limits.
type SRPNativeConfig struct {
	// AppPaths scopes the check to files under these path prefixes. Empty = all.
	AppPaths []string `json:"appPaths"`
	// ExcludePaths skips files whose path contains any of these substrings.
	// Build artifacts and vendored sources are ALWAYS excluded on top of these
	// (see defaultNativeExcludes) so a giant dependency file can never leak in.
	ExcludePaths []string `json:"excludePaths"`
	// SwiftExtensions / KotlinExtensions select the grammar per file.
	SwiftExtensions  []string `json:"swiftExtensions"`
	KotlinExtensions []string `json:"kotlinExtensions"`
	// Line-count limits. Zero ⇒ srpnative package defaults (400/300/60).
	FileLines     int `json:"fileLines"`
	TypeBodyLines int `json:"typeBodyLines"`
	FuncBodyLines int `json:"funcBodyLines"`
	// MinTypeBodyLines is the body-size floor for the oneTypePerFile rule: a
	// top-level type only counts toward the one-type-per-file limit if its body
	// spans at least this many lines, so small co-located helper types (SwiftUI
	// subviews, response DTOs) don't force a file split. Zero ⇒ default (40).
	MinTypeBodyLines int `json:"minTypeBodyLines"`
	// OneTypeIgnoreConformances names protocols whose conformers are exempt from
	// the oneTypePerFile count. Empty ⇒ default ["PreviewProvider"]; provide a
	// list to extend or replace it.
	OneTypeIgnoreConformances []string `json:"oneTypeIgnoreConformances"`
	// FileLinesOverrides maps a path substring to a per-file line limit.
	FileLinesOverrides map[string]int `json:"fileLinesOverrides"`
	// EnabledRules limits which detectors run. Empty = all four.
	EnabledRules []string `json:"enabledRules"`
}

// defaultNativeExcludes are build/vendored directories that must never be
// scanned regardless of project config — SwiftPM checkouts, Xcode build output,
// CocoaPods, and Gradle artifacts hold huge generated files.
var defaultNativeExcludes = []string{
	"/build/", "/build-device/", "/.build/", "/DerivedData/",
	"/SourcePackages/", "/Pods/", "/.gradle/", "/node_modules/",
}

func (c SRPNativeConfig) resolved() SRPNativeConfig {
	if len(c.SwiftExtensions) == 0 {
		c.SwiftExtensions = []string{".swift"}
	}
	if len(c.KotlinExtensions) == 0 {
		c.KotlinExtensions = []string{".kt", ".kts"}
	}
	c.ExcludePaths = append(append([]string{}, defaultNativeExcludes...), c.ExcludePaths...)
	return c
}

// langForFile returns the grammar for a path's extension, or false if the file
// isn't a native source file in scope.
func (c SRPNativeConfig) langForFile(file string) (srpnative.Lang, bool) {
	lower := strings.ToLower(file)
	for _, ext := range c.SwiftExtensions {
		if strings.HasSuffix(lower, ext) {
			return srpnative.Swift, true
		}
	}
	for _, ext := range c.KotlinExtensions {
		if strings.HasSuffix(lower, ext) {
			return srpnative.Kotlin, true
		}
	}
	return srpnative.Swift, false
}

func (c SRPNativeConfig) inScope(file string) bool {
	for _, ex := range c.ExcludePaths {
		if ex != "" && strings.Contains(file, ex) {
			return false
		}
	}
	if len(c.AppPaths) == 0 {
		return true
	}
	for _, p := range c.AppPaths {
		prefix := strings.TrimSuffix(p, "/") + "/"
		if strings.HasPrefix(file, prefix) {
			return true
		}
	}
	return false
}

func (c SRPNativeConfig) options() srpnative.Options {
	enabled := map[string]bool{}
	for _, r := range c.EnabledRules {
		enabled[r] = true
	}
	var ignoreConf map[string]bool
	if len(c.OneTypeIgnoreConformances) > 0 {
		ignoreConf = map[string]bool{}
		for _, name := range c.OneTypeIgnoreConformances {
			ignoreConf[name] = true
		}
	}
	return srpnative.Options{
		FileLines:                 c.FileLines,
		TypeBodyLines:             c.TypeBodyLines,
		FuncBodyLines:             c.FuncBodyLines,
		MinTypeBodyLines:          c.MinTypeBodyLines,
		OneTypeIgnoreConformances: ignoreConf, // nil ⇒ package default {"PreviewProvider"}
		FileLinesOverrides:        c.FileLinesOverrides,
		EnabledRules:              enabled,
	}
}

// readNativeSource returns the staged content of a file (git show :path), or
// falls back to the working-tree file for standalone/untracked/absolute paths.
func readNativeSource(file string) ([]byte, error) {
	if !filepath.IsAbs(file) {
		if out, err := exec.Command("git", "show", ":"+file).Output(); err == nil {
			return out, nil
		}
	}
	return os.ReadFile(file)
}

// runSRPNativeCheck analyzes staged native source files and blocks the commit on
// any violation. All native violations are errors (hard-error policy); there is
// no warnOnly/grandfather path — scoping is by staged files only.
func runSRPNativeCheck(files []string, cfg SRPNativeConfig) error {
	cfg = cfg.resolved()
	opts := cfg.options()

	var scanned int
	var violations []srpnative.Violation
	for _, file := range files {
		lang, ok := cfg.langForFile(file)
		if !ok || !cfg.inScope(file) {
			continue
		}
		content, err := readNativeSource(file)
		if err != nil {
			continue
		}
		scanned++
		a := srpnative.Analyze(string(content), file, lang)
		violations = append(violations, srpnative.RunDetectors(a, file, opts)...)
	}

	if reportDir != "" && len(violations) > 0 {
		if err := writeSRPNativeReport(violations, reportDir); err != nil {
			fmt.Printf("   Warning: failed to write SRP native report: %v\n", err)
		}
	} else if reportDir != "" {
		// Always-write: leave a passing fullreport.txt on a clean run.
		_ = writeRunReport("srpNative", "SRP native compliance", "", false)
	}

	if compactMode() {
		if len(violations) > 0 {
			appCounts := map[string]int{}
			for _, v := range violations {
				appCounts[getSRPAppNameFromPath(v.File)]++
			}
			apps := make([]string, 0, len(appCounts))
			for app := range appCounts {
				apps = append(apps, app)
			}
			sort.Strings(apps)
			parts := make([]string, len(apps))
			for i, app := range apps {
				parts[i] = fmt.Sprintf("%s %d errors", app, appCounts[app])
			}
			printStatus("SRP native compliance", false, strings.Join(parts, ", "))
			printReportHint("srpNative/")
			return fmt.Errorf("native SRP violations found")
		}
		printStatus("SRP native compliance", true, fmt.Sprintf("%d files", scanned))
		return nil
	}

	for _, v := range violations {
		fmt.Printf("❌ %s: %s\n", v.File, v.Message)
		if v.Suggestion != "" {
			fmt.Printf("   FIX: %s\n", v.Suggestion)
		}
	}
	if len(violations) > 0 {
		fmt.Printf("\n❌ Found %d native SRP violation(s)\n\n", len(violations))
		return fmt.Errorf("native SRP violations found")
	}
	fmt.Println("✅ SRP native check passed")
	return nil
}

// writeSRPNativeReport writes native SRP findings grouped by app, mirroring
// writeSRPReport's layout under reportDir/srpNative/<app>.txt.
func writeSRPNativeReport(violations []srpnative.Violation, baseDir string) error {
	dir := filepath.Join(baseDir, "srpNative")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	byApp := map[string][]srpnative.Violation{}
	for _, v := range violations {
		app := getSRPAppNameFromPath(v.File)
		byApp[app] = append(byApp[app], v)
	}

	for app, vs := range byApp {
		var sb strings.Builder
		sb.WriteString(strings.Repeat("=", 80) + "\n")
		fmt.Fprintf(&sb, "SRP NATIVE ANALYSIS - %s\n", strings.ToUpper(app))
		fmt.Fprintf(&sb, "Generated: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		sb.WriteString(strings.Repeat("=", 80) + "\n\n")
		fmt.Fprintf(&sb, "Total errors: %d\n\n", len(vs))

		byFile := map[string][]srpnative.Violation{}
		for _, v := range vs {
			byFile[v.File] = append(byFile[v.File], v)
		}
		files := make([]string, 0, len(byFile))
		for f := range byFile {
			files = append(files, f)
		}
		sort.Strings(files)
		for _, f := range files {
			fmt.Fprintf(&sb, "\n%s (%d issues)\n", f, len(byFile[f]))
			for _, v := range byFile[f] {
				fmt.Fprintf(&sb, "  ❌ [%s] %s\n", v.RuleID, v.Message)
				if v.Suggestion != "" {
					fmt.Fprintf(&sb, "     → %s\n", v.Suggestion)
				}
			}
		}
		// Findings-only report: flat "file: [rule] message" list.
		var findingsBody strings.Builder
		for _, f := range files {
			for _, v := range byFile[f] {
				fmt.Fprintf(&findingsBody, "  %s: [%s] %s\n", f, v.RuleID, v.Message)
			}
		}
		findings := findingsDoc("SRP NATIVE", app, len(vs), findingsBody.String())

		if err := writeDualReport(baseDir, "srpNative", app, findings, sb.String()); err != nil {
			return err
		}
	}
	return nil
}
