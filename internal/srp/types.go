// Package srp is the shared Single Responsibility Principle analyzer used by
// both the pre-commit orchestrator (cmd/pre-commit) and the standalone /
// hook-mode validator (cmd/validate-srp). It parses TypeScript/TSX with
// tree-sitter (the same engine internal/stubs uses) and runs six structural
// detectors. Keeping one implementation here means the two entry points can
// never drift in what they flag.
package srp

// Violation is one SRP finding. Severity is the detector's default; callers
// (the orchestrator) may downgrade it via their own warnOnly/errorScope rules.
type Violation struct {
	File       string
	Severity   string // "error" or "warning"
	Message    string
	Suggestion string
	RuleID     string
}

// Analysis is the structural summary of one source file.
type Analysis struct {
	FilePath                 string
	Imports                  []ImportInfo
	Exports                  []ExportInfo
	StateManagement          []StateInfo
	LineCount                int
	HasResponsibilityComment bool
}

// ImportInfo is one import statement: the module source and the imported names.
type ImportInfo struct {
	Source string
	Names  []string
}

// ExportInfo is one named/declaration export.
type ExportInfo struct {
	Name       string
	Type       string // const|let|var|function|class|type|interface|default
	IsTypeOnly bool
	Source     string // re-export source module, if any
}

// StateInfo is one React state-hook call site.
type StateInfo struct {
	Hook string
	Line int
}

// Options tunes the detectors. The zero value is valid (defaults applied).
type Options struct {
	// ScreenHooks is the set of hooks that count as state in screens/pages.
	// Empty → useState/useReducer/useContext.
	ScreenHooks map[string]bool
	// EnabledRules limits which detectors run. nil/empty → all six.
	EnabledRules map[string]bool
}

var defaultScreenHooks = map[string]bool{
	"useState": true, "useReducer": true, "useContext": true,
}

func (o Options) screenHooks() map[string]bool {
	if len(o.ScreenHooks) == 0 {
		return defaultScreenHooks
	}
	return o.ScreenHooks
}

func (o Options) ruleEnabled(id string) bool {
	if len(o.EnabledRules) == 0 {
		return true
	}
	return o.EnabledRules[id]
}
