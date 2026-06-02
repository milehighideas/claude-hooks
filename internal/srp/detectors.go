package srp

import (
	"fmt"
	"strings"
)

// RunDetectors runs the six structural SRP detectors against an analysis and
// returns their violations with default severities. Callers apply their own
// severity policy (warnOnly / errorScopes / warningOnlyPaths) afterward.
func RunDetectors(a *Analysis, filePath string, opts Options) []Violation {
	var v []Violation
	if opts.ruleEnabled("directConvexImports") {
		v = append(v, checkDirectConvexImports(a, filePath)...)
	}
	if opts.ruleEnabled("stateInScreens") {
		v = append(v, checkStateInScreens(a, filePath, opts.screenHooks())...)
	}
	if opts.ruleEnabled("multipleExports") {
		v = append(v, checkMultipleExports(a, filePath)...)
	}
	if opts.ruleEnabled("fileSize") {
		v = append(v, checkFileSize(a, filePath)...)
	}
	if opts.ruleEnabled("typeExportsLocation") {
		v = append(v, checkTypeExportsLocation(a, filePath)...)
	}
	if opts.ruleEnabled("mixedConcerns") {
		v = append(v, checkMixedConcerns(a, filePath)...)
	}
	return v
}

// isScreenOrPage reports whether the file is a mobile screen or a Next.js page
// — thin routing layers that should hold no state.
func isScreenOrPage(filePath string) bool {
	return strings.Contains(filePath, "/screens/") || strings.HasSuffix(filePath, "page.tsx")
}

func checkDirectConvexImports(a *Analysis, filePath string) []Violation {
	var v []Violation
	if strings.Contains(filePath, "/data-layer/") ||
		strings.Contains(filePath, "/backend/") ||
		strings.Contains(filePath, "/convex/") ||
		strings.Contains(filePath, "/scripts/") ||
		strings.Contains(filePath, "/providers/") ||
		strings.HasSuffix(filePath, "_layout.tsx") {
		return v
	}

	allowedImports := map[string]bool{"Preloaded": true, "usePreloadedQuery": true}
	allowedDataModelTypes := map[string]bool{"Id": true, "Doc": true}

	for _, imp := range a.Imports {
		if imp.Source == "convex/react" {
			for _, name := range imp.Names {
				if !allowedImports[name] {
					v = append(v, Violation{
						File:       filePath,
						Severity:   "error",
						Message:    "Direct Convex imports forbidden outside data-layer",
						Suggestion: "Use data-layer hooks instead",
						RuleID:     "directConvexImports",
					})
					break
				}
			}
		}
		if strings.Contains(imp.Source, "_generated/api") {
			v = append(v, Violation{
				File:       filePath,
				Severity:   "error",
				Message:    "Direct Convex API imports forbidden outside data-layer",
				Suggestion: "Use data-layer hooks instead",
				RuleID:     "directConvexImports",
			})
		}
		if strings.Contains(imp.Source, "_generated/dataModel") {
			for _, name := range imp.Names {
				clean := strings.TrimPrefix(name, "type ")
				if !allowedDataModelTypes[clean] {
					v = append(v, Violation{
						File:       filePath,
						Severity:   "error",
						Message:    fmt.Sprintf("Only Id and Doc types allowed from _generated/dataModel, found: %s", name),
						Suggestion: "Use data-layer types instead, or import only Id/Doc",
						RuleID:     "directConvexImports",
					})
					break
				}
			}
		}
	}
	return v
}

func checkStateInScreens(a *Analysis, filePath string, allowedHooks map[string]bool) []Violation {
	var v []Violation
	if !isScreenOrPage(filePath) {
		return v
	}
	var flagged []string
	for _, s := range a.StateManagement {
		if allowedHooks[s.Hook] {
			flagged = append(flagged, s.Hook)
		}
	}
	if len(flagged) > 0 {
		fileType := "Screen"
		if strings.HasSuffix(filePath, "page.tsx") {
			fileType = "Page"
		}
		v = append(v, Violation{
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("%s has state management (%s)", fileType, strings.Join(flagged, ", ")),
			Suggestion: "Move state to content component or hook - screens are navigation-only",
			RuleID:     "stateInScreens",
		})
	}
	return v
}

func checkMultipleExports(a *Analysis, filePath string) []Violation {
	var v []Violation
	hasCRUD := false
	for _, folder := range []string{"/create/", "/read/", "/update/", "/delete/"} {
		if strings.Contains(filePath, folder) {
			hasCRUD = true
			break
		}
	}
	if !hasCRUD {
		return v
	}
	nonType := 0
	for _, e := range a.Exports {
		if !e.IsTypeOnly && e.Type != "type" && e.Type != "interface" {
			nonType++
		}
	}
	if nonType > 1 {
		v = append(v, Violation{
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("Multiple exports (%d) in CRUD component", nonType),
			Suggestion: "Split into separate files (one component per file)",
			RuleID:     "multipleExports",
		})
	}
	return v
}

func checkFileSize(a *Analysis, filePath string) []Violation {
	var v []Violation
	if strings.Contains(filePath, "/scripts/") {
		return v
	}
	limits := map[string]int{"screen": 100, "hook": 150, "component": 200}
	n := a.LineCount

	switch {
	case isScreenOrPage(filePath) && n > limits["screen"]:
		fileType := "Screen"
		if strings.HasSuffix(filePath, "page.tsx") {
			fileType = "Page"
		}
		v = append(v, Violation{
			File: filePath, Severity: "warning",
			Message:    fmt.Sprintf("%s file is %d lines (limit: %d)", fileType, n, limits["screen"]),
			Suggestion: "Move logic to content component",
			RuleID:     "fileSize",
		})
	case strings.Contains(filePath, "/hooks/") && n > limits["hook"]:
		v = append(v, Violation{
			File: filePath, Severity: "warning",
			Message:    fmt.Sprintf("Hook file is %d lines (limit: %d)", n, limits["hook"]),
			Suggestion: "Split into smaller hooks",
			RuleID:     "fileSize",
		})
	case n > limits["component"]:
		v = append(v, Violation{
			File: filePath, Severity: "warning",
			Message:    fmt.Sprintf("File is %d lines (limit: %d)", n, limits["component"]),
			Suggestion: "Consider splitting",
			RuleID:     "fileSize",
		})
	}
	return v
}

func checkTypeExportsLocation(a *Analysis, filePath string) []Violation {
	var v []Violation
	if strings.Contains(filePath, "/types/") ||
		strings.HasSuffix(filePath, ".d.ts") ||
		strings.Contains(filePath, "/generated-types/") ||
		strings.Contains(filePath, "/data-layer/") ||
		strings.Contains(filePath, "packages/ui/") ||
		strings.Contains(filePath, "packages/mobile-ui/") {
		return v
	}
	for _, e := range a.Exports {
		if e.IsTypeOnly || e.Type == "type" || e.Type == "interface" {
			if strings.HasSuffix(e.Name, "Props") {
				continue
			}
			v = append(v, Violation{
				File:       filePath,
				Severity:   "error",
				Message:    fmt.Sprintf("Type export '%s' found outside types/ folder", e.Name),
				Suggestion: "Move type definitions to types/ folder",
				RuleID:     "typeExportsLocation",
			})
		}
	}
	return v
}

func checkMixedConcerns(a *Analysis, filePath string) []Violation {
	var v []Violation
	stateOnly := map[string]bool{"useState": true, "useReducer": true, "useContext": true}
	hasState := false
	for _, s := range a.StateManagement {
		if stateOnly[s.Hook] {
			hasState = true
			break
		}
	}
	hasDataLayer, hasUI := false, false
	for _, imp := range a.Imports {
		if strings.Contains(imp.Source, "data-layer") {
			hasDataLayer = true
		}
		if strings.Contains(imp.Source, "@/components/ui") ||
			strings.Contains(imp.Source, "../ui/") ||
			strings.Contains(imp.Source, "@dashtag/ui") ||
			strings.Contains(imp.Source, "@dashtag/mobile-ui") {
			hasUI = true
		}
	}
	var concerns []string
	if hasDataLayer {
		concerns = append(concerns, "data fetching")
	}
	if hasUI {
		concerns = append(concerns, "UI components")
	}
	if hasState {
		concerns = append(concerns, "state management")
	}
	if len(concerns) >= 3 {
		v = append(v, Violation{
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("File mixes multiple concerns: %s", strings.Join(concerns, ", ")),
			Suggestion: "Separate data fetching (hooks), state (hooks/content), and UI (components)",
			RuleID:     "mixedConcerns",
		})
	}
	return v
}
