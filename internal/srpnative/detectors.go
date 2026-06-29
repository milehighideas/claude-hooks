package srpnative

import "fmt"

// RunDetectors runs the four native structural SRP detectors and returns their
// violations. All native violations default to "error" severity; the pre-commit
// orchestrator applies its own path-based severity policy afterward.
func RunDetectors(a *Analysis, filePath string, opts Options) []Violation {
	opts = opts.WithDefaults()
	var v []Violation
	if opts.ruleEnabled("fileSize") {
		v = append(v, checkFileSize(a, filePath, opts)...)
	}
	if opts.ruleEnabled("typeBodyLength") {
		v = append(v, checkTypeBodyLength(a, filePath, opts)...)
	}
	if opts.ruleEnabled("functionBodyLength") {
		v = append(v, checkFunctionBodyLength(a, filePath, opts)...)
	}
	if opts.ruleEnabled("oneTypePerFile") {
		v = append(v, checkOneTypePerFile(a, filePath, opts.MinTypeBodyLines, opts.OneTypeIgnoreConformances)...)
	}
	return v
}

func checkFileSize(a *Analysis, filePath string, opts Options) []Violation {
	limit := opts.fileLineLimit(filePath)
	if a.LineCount <= limit {
		return nil
	}
	return []Violation{{
		File:       filePath,
		Severity:   "error",
		Message:    fmt.Sprintf("File is %d lines (limit: %d)", a.LineCount, limit),
		Suggestion: "Split into focused files — extract types, extensions, or helpers",
		RuleID:     "fileSize",
	}}
}

func checkTypeBodyLength(a *Analysis, filePath string, opts Options) []Violation {
	var v []Violation
	for _, t := range a.Types {
		if t.BodyLines <= opts.TypeBodyLines {
			continue
		}
		label := t.Kind
		if t.Name != "" {
			label = fmt.Sprintf("%s '%s'", t.Kind, t.Name)
		}
		v = append(v, Violation{
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("%s body is %d lines (limit: %d) at line %d", label, t.BodyLines, opts.TypeBodyLines, t.StartLine),
			Suggestion: "Break the type apart — move responsibilities into separate types or extensions",
			RuleID:     "typeBodyLength",
		})
	}
	return v
}

func checkFunctionBodyLength(a *Analysis, filePath string, opts Options) []Violation {
	var v []Violation
	for _, f := range a.Funcs {
		if f.BodyLines <= opts.FuncBodyLines {
			continue
		}
		label := "function"
		if f.Name != "" {
			label = fmt.Sprintf("function '%s'", f.Name)
		}
		v = append(v, Violation{
			File:       filePath,
			Severity:   "error",
			Message:    fmt.Sprintf("%s body is %d lines (limit: %d) at line %d", label, f.BodyLines, opts.FuncBodyLines, f.StartLine),
			Suggestion: "Extract sub-steps into smaller functions",
			RuleID:     "functionBodyLength",
		})
	}
	return v
}

// checkOneTypePerFile flags a file declaring more than one SUBSTANTIAL top-level
// type. Excluded from the count:
//   - Swift extensions (co-locating a type with its own extensions is idiomatic).
//   - Nested types (only top-level declarations count).
//   - Trivial types whose body is under minTypeBodyLines — small private SwiftUI
//     subviews, helper rows, and grouped response DTOs aren't separate
//     responsibilities and shouldn't force a file split.
//   - Types conforming to an ignored protocol (e.g. PreviewProvider) — Xcode
//     preview scaffolding co-located with its view is not a responsibility.
func checkOneTypePerFile(a *Analysis, filePath string, minTypeBodyLines int, ignoreConformances map[string]bool) []Violation {
	var names []string
	count := 0
	for _, t := range a.Types {
		if !t.IsTopLevel || t.IsExtension || t.BodyLines < minTypeBodyLines {
			continue
		}
		if conformsToAny(t.Conformances, ignoreConformances) {
			continue
		}
		count++
		if t.Name != "" {
			names = append(names, t.Name)
		}
	}
	if count <= 1 {
		return nil
	}
	detail := ""
	if len(names) > 0 {
		detail = ": " + joinComma(names)
	}
	return []Violation{{
		File:       filePath,
		Severity:   "error",
		Message:    fmt.Sprintf("File declares %d top-level types%s", count, detail),
		Suggestion: "Move each type to its own file (one type per file)",
		RuleID:     "oneTypePerFile",
	}}
}

func conformsToAny(conformances []string, ignore map[string]bool) bool {
	if len(ignore) == 0 {
		return false
	}
	for _, c := range conformances {
		if ignore[c] {
			return true
		}
	}
	return false
}

func joinComma(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}
