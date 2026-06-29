// Package srpnative is the structural Single Responsibility Principle analyzer
// for native source — Swift (iOS) and Kotlin (Android). It is the native
// counterpart to internal/srp (which handles TypeScript/TSX): same tree-sitter
// engine (github.com/smacker/go-tree-sitter), same Violation shape, but a
// language-parameterized parser and a native-appropriate rule set.
//
// Four detectors run against the parsed AST:
//   - fileSize           total file lines over a per-file limit
//   - typeBodyLength     a single type body (the Massive-View-Controller smell)
//   - functionBodyLength a single function/method body
//   - oneTypePerFile     more than one top-level type declaration in a file
//
// The TypeScript-only rules from internal/srp (directConvexImports,
// typeExportsLocation, stateInScreens, mixedConcerns) do not port and are
// intentionally absent.
package srpnative

import "strings"

// Lang selects the tree-sitter grammar to parse with.
type Lang int

const (
	Swift Lang = iota
	Kotlin
)

// Violation is one SRP finding. Field shape is identical to internal/srp.Violation
// so the pre-commit orchestrator can convert between them with a struct cast.
type Violation struct {
	File       string
	Severity   string // "error" or "warning"
	Message    string
	Suggestion string
	RuleID     string
}

// Analysis is the structural summary of one native source file.
type Analysis struct {
	FilePath  string
	Lang      Lang
	LineCount int
	Types     []TypeDecl
	Funcs     []FuncDecl
}

// TypeDecl is one type declaration (class/struct/enum/actor/protocol/object/interface).
type TypeDecl struct {
	Kind         string // the declaration keyword: "class","struct","enum","actor","extension","protocol","object","interface"
	Name         string
	IsExtension  bool     // Swift `extension Foo {}` — excluded from oneTypePerFile
	IsTopLevel   bool     // parent is the source file (not nested inside another type)
	StartLine    int      // 1-based line of the declaration keyword
	BodyLines    int      // lines spanned by the type body (0 if no brace body)
	Conformances []string // protocols/base types this declaration inherits from
}

// FuncDecl is one function/method declaration with a brace body.
type FuncDecl struct {
	Name      string
	StartLine int // 1-based
	BodyLines int // lines spanned by the function body
}

// Options tunes the detectors. The zero value is NOT meant to be used directly;
// callers pass resolved thresholds. WithDefaults fills any unset numeric limit.
type Options struct {
	FileLines     int // total-file line limit
	TypeBodyLines int // single-type body line limit
	FuncBodyLines int // single-function body line limit
	// MinTypeBodyLines is the body-size floor for oneTypePerFile: a top-level
	// type only counts toward the one-type-per-file limit if its body spans at
	// least this many lines. This keeps the rule from flagging idiomatic
	// co-location — a parent SwiftUI view with small private subviews, or a file
	// grouping several tiny response DTOs — while still catching files that pack
	// multiple substantial responsibilities together.
	MinTypeBodyLines int
	// OneTypeIgnoreConformances names protocols/base types whose conformers are
	// excluded from the oneTypePerFile count. Defaults to {"PreviewProvider"} —
	// an Xcode preview struct co-located with its view is tooling scaffolding,
	// not a second responsibility.
	OneTypeIgnoreConformances map[string]bool
	FileLinesOverrides        map[string]int // path-substring -> file-line limit override
	EnabledRules              map[string]bool
}

const (
	defaultFileLines        = 400
	defaultTypeBodyLines    = 300
	defaultFuncBodyLines    = 60
	defaultMinTypeBodyLines = 40
)

// WithDefaults returns a copy with any zero numeric limit filled from the
// package defaults, so a partially-specified config still behaves sanely.
func (o Options) WithDefaults() Options {
	if o.FileLines <= 0 {
		o.FileLines = defaultFileLines
	}
	if o.TypeBodyLines <= 0 {
		o.TypeBodyLines = defaultTypeBodyLines
	}
	if o.FuncBodyLines <= 0 {
		o.FuncBodyLines = defaultFuncBodyLines
	}
	if o.MinTypeBodyLines <= 0 {
		o.MinTypeBodyLines = defaultMinTypeBodyLines
	}
	if o.OneTypeIgnoreConformances == nil {
		o.OneTypeIgnoreConformances = map[string]bool{"PreviewProvider": true}
	}
	return o
}

func (o Options) ruleEnabled(id string) bool {
	if len(o.EnabledRules) == 0 {
		return true
	}
	return o.EnabledRules[id]
}

// fileLineLimit returns the file-line limit for a path, honoring the first
// matching substring override before falling back to the flat FileLines limit.
func (o Options) fileLineLimit(filePath string) int {
	for sub, lim := range o.FileLinesOverrides {
		if sub != "" && lim > 0 && strings.Contains(filePath, sub) {
			return lim
		}
	}
	return o.FileLines
}
