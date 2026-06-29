package srpnative

import "testing"

func ruleIDs(v []Violation) map[string]int {
	m := map[string]int{}
	for _, x := range v {
		m[x.RuleID]++
	}
	return m
}

func TestDetectors(t *testing.T) {
	tests := []struct {
		name string
		lang Lang
		code string
		opts Options
		want map[string]int // ruleID -> expected count; absent rule ⇒ 0
	}{
		{
			name: "swift: single class + its extension is fine",
			lang: Swift,
			code: `import Foo
class Widget {
    func a() {}
}
extension Widget {
    func b() {}
}
`,
			opts: Options{},
			want: map[string]int{},
		},
		{
			name: "swift: two substantial top-level types flagged",
			lang: Swift,
			code: `class A {
    func one() {}
    func two() {}
    func three() {}
}
class B {
    func four() {}
    func five() {}
    func six() {}
}
`,
			opts: Options{MinTypeBodyLines: 3},
			want: map[string]int{"oneTypePerFile": 1},
		},
		{
			name: "swift: substantial type + trivial helper is fine",
			lang: Swift,
			code: `struct HomeView {
    var body: Int {
        return 1
    }
    func render() {}
    func layout() {}
}
struct FeatureRow { let title = "x" }
`,
			opts: Options{MinTypeBodyLines: 3},
			want: map[string]int{},
		},
		{
			name: "swift: view + its PreviewProvider is fine",
			lang: Swift,
			code: `struct TopNavBar: View {
    var body: some View {
        Text("nav")
    }
    func a() {}
    func b() {}
}
struct TopNavBarPreview: PreviewProvider {
    static var previews: some View {
        TopNavBar()
    }
    static var more: Int { 1 }
    static var extra: Int { 2 }
}
`,
			opts: Options{MinTypeBodyLines: 3},
			want: map[string]int{},
		},
		{
			name: "swift: several trivial DTOs are fine",
			lang: Swift,
			code: `struct Resp1 { let a = 1 }
struct Resp2 { let b = 2 }
struct Resp3 { let c = 3 }
`,
			opts: Options{MinTypeBodyLines: 3},
			want: map[string]int{},
		},
		{
			name: "swift: type body over limit",
			lang: Swift,
			code: `class Big {
    let a = 1
    let b = 2
    let c = 3
    let d = 4
    let e = 5
}
`,
			opts: Options{TypeBodyLines: 4},
			want: map[string]int{"typeBodyLength": 1},
		},
		{
			name: "swift: function body over limit",
			lang: Swift,
			code: `class C {
    func work() {
        let a = 1
        let b = 2
        let c = 3
        let d = 4
    }
}
`,
			opts: Options{FuncBodyLines: 3},
			want: map[string]int{"functionBodyLength": 1},
		},
		{
			name: "swift: file over limit",
			lang: Swift,
			code: "class C {\n    func a() {}\n}\n// l1\n// l2\n// l3\n// l4\n",
			opts: Options{FileLines: 5},
			want: map[string]int{"fileSize": 1},
		},
		{
			name: "swift: nested type does not trigger oneTypePerFile",
			lang: Swift,
			code: `class Outer {
    struct Inner { let x = 1 }
}
`,
			opts: Options{},
			want: map[string]int{},
		},
		{
			name: "kotlin: class + object are two types",
			lang: Kotlin,
			code: `package x

class A {
    fun m() {}
}

object B {
    val y = 2
}
`,
			opts: Options{MinTypeBodyLines: 2},
			want: map[string]int{"oneTypePerFile": 1},
		},
		{
			name: "kotlin: single class is fine",
			lang: Kotlin,
			code: `package x

class A {
    fun m() {}
}
`,
			opts: Options{},
			want: map[string]int{},
		},
		{
			name: "kotlin: function body over limit",
			lang: Kotlin,
			code: `package x

class A {
    fun work() {
        val a = 1
        val b = 2
        val c = 3
        val d = 4
    }
}
`,
			opts: Options{FuncBodyLines: 3},
			want: map[string]int{"functionBodyLength": 1},
		},
		{
			name: "clean file: no violations under generous defaults",
			lang: Swift,
			code: `class Small {
    func a() { let x = 1 }
}
`,
			opts: Options{},
			want: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Analyze(tt.code, "apps/dashtag-maps-ios/App/Test.swift", tt.lang)
			got := ruleIDs(RunDetectors(a, a.FilePath, tt.opts))
			for _, rule := range []string{"fileSize", "typeBodyLength", "functionBodyLength", "oneTypePerFile"} {
				if got[rule] != tt.want[rule] {
					t.Errorf("rule %q: got %d, want %d (all: %+v)", rule, got[rule], tt.want[rule], got)
				}
			}
		})
	}
}

// TestExtensionKindDetection locks in the load-bearing fact that Swift
// extensions parse as class_declaration but are excluded from the type count.
func TestExtensionKindDetection(t *testing.T) {
	a := Analyze("extension Foo {\n    func b() {}\n}\n", "x.swift", Swift)
	if len(a.Types) != 1 {
		t.Fatalf("want 1 type decl, got %d", len(a.Types))
	}
	if !a.Types[0].IsExtension || a.Types[0].Kind != "extension" {
		t.Fatalf("extension not detected: %+v", a.Types[0])
	}
}
