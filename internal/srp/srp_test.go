package srp

import "testing"

func ruleIDs(vs []Violation) map[string]int {
	m := map[string]int{}
	for _, v := range vs {
		m[v.RuleID]++
	}
	return m
}

func TestMultilineConvexImport(t *testing.T) {
	// The old line-based scanner missed multiline imports; the AST parser
	// resolves the whole import_statement node.
	code := `import {
  useQuery,
  useMutation,
} from "convex/react";

export function Foo() { return null }
`
	a := Analyze(code, "apps/mobile/components/foo.tsx")
	if len(a.Imports) != 1 || a.Imports[0].Source != "convex/react" {
		t.Fatalf("imports = %+v", a.Imports)
	}
	v := RunDetectors(a, "apps/mobile/components/foo.tsx", Options{})
	if ruleIDs(v)["directConvexImports"] != 1 {
		t.Fatalf("want directConvexImports, got %+v", v)
	}
}

func TestStateInPage(t *testing.T) {
	code := `import { useState } from "react";
export default function Page() { const [x,setX] = useState(0); return null }`
	v := RunDetectors(Analyze(code, "apps/web/app/about/page.tsx"), "apps/web/app/about/page.tsx", Options{})
	if ruleIDs(v)["stateInScreens"] != 1 {
		t.Fatalf("want stateInScreens for page.tsx, got %+v", v)
	}
}

func TestTypeExportExemptsProps(t *testing.T) {
	code := `export type FooProps = { a: number };
export type Bar = { b: string };`
	v := RunDetectors(Analyze(code, "apps/web/components/foo.tsx"), "apps/web/components/foo.tsx", Options{})
	// FooProps is exempt; Bar is flagged.
	if got := ruleIDs(v)["typeExportsLocation"]; got != 1 {
		t.Fatalf("want 1 typeExportsLocation (Bar only), got %d: %+v", got, v)
	}
}

func TestMultipleExportsIgnoresTypes(t *testing.T) {
	code := `export function A(){return null}
export function B(){return null}
export type T = number;`
	v := RunDetectors(Analyze(code, "apps/web/components/read/widget.tsx"), "apps/web/components/read/widget.tsx", Options{})
	if ruleIDs(v)["multipleExports"] != 1 {
		t.Fatalf("want multipleExports (2 fns), got %+v", v)
	}
}

func TestMixedConcernsIsError(t *testing.T) {
	code := `import { useUser } from "@dashtag/data-layer/hooks";
import { Button } from "@dashtag/ui";
import { useState } from "react";
export function Screen(){ const [x,setX]=useState(0); return <Button/> }`
	v := RunDetectors(Analyze(code, "apps/web/components/x.tsx"), "apps/web/components/x.tsx", Options{})
	var sev string
	for _, vi := range v {
		if vi.RuleID == "mixedConcerns" {
			sev = vi.Severity
		}
	}
	if sev != "error" {
		t.Fatalf("want mixedConcerns error, got sev=%q all=%+v", sev, v)
	}
}

func TestEnabledRulesFilter(t *testing.T) {
	code := `import { useQuery } from "convex/react";`
	v := RunDetectors(Analyze(code, "apps/web/components/x.tsx"), "apps/web/components/x.tsx",
		Options{EnabledRules: map[string]bool{"fileSize": true}})
	if len(v) != 0 {
		t.Fatalf("only fileSize enabled, convex import should not flag: %+v", v)
	}
}

func TestParseErrorFailsOpen(t *testing.T) {
	a := Analyze("import { from 'broken", "x.tsx")
	if a == nil || a.LineCount == 0 {
		t.Fatal("analysis should be non-nil with LineCount on parse error")
	}
}
