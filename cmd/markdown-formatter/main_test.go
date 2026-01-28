package main

import (
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		// JSON
		{"json object", `{"key": "value", "num": 123}`, "json"},
		{"json array", `[1, 2, 3, "test"]`, "json"},
		{"json nested", `{"users": [{"name": "John"}]}`, "json"},

		// Python
		{"python function", "def hello():\n    print('hello')", "python"},
		{"python import", "import os\nimport sys", "python"},
		{"python from import", "from typing import List", "python"},

		// Go
		{"go package", "package main\n\nfunc main() {}", "go"},
		{"go func", "func doSomething(x int) error {\n\treturn nil\n}", "go"},
		{"go import", `import "fmt"`, "go"},
		{"go multi import", "import (\n\t\"fmt\"\n\t\"os\"\n)", "go"},

		// Rust
		{"rust fn", "fn main() {\n    println!(\"Hello\");\n}", "rust"},
		{"rust pub fn", "pub fn new() -> Self {\n    Self {}\n}", "rust"},
		{"rust impl", "impl MyStruct {\n    fn method(&self) {}\n}", "rust"},
		{"rust let mut", "let mut x = 5;\nx += 1;", "rust"},

		// TypeScript
		{"typescript interface", "interface User {\n  name: string;\n}", "typescript"},
		{"typescript type", "type Props = {\n  id: number;\n}", "typescript"},
		{"typescript generic", "interface Config {}\nconst arr: Array<string> = [];", "typescript"},

		// JavaScript
		{"javascript function", "function hello() {\n  console.log('hi');\n}", "javascript"},
		{"javascript const", "const x = 5;\nconst y = 10;", "javascript"},
		{"javascript arrow", "const fn = () => { return 1; };", "javascript"},
		{"javascript console", "console.log('test');\nconsole.error('err');", "javascript"},

		// JSX/TSX
		{"jsx component", "<MyComponent className=\"test\" />", "jsx"},
		{"tsx component", "<Component className=\"x\"><Child /></Component>", "jsx"},

		// Bash
		{"bash shebang", "#!/bin/bash\necho hello", "bash"},
		{"bash control", "if [ -f file ]; then\n  echo exists\nfi", "bash"},
		{"bash commands", "mkdir -p dir\ncd dir\nls -la", "bash"},

		// SQL
		{"sql select", "SELECT * FROM users WHERE id = 1", "sql"},
		{"sql insert", "INSERT INTO users (name) VALUES ('John')", "sql"},
		{"sql create", "CREATE TABLE users (\n  id INT PRIMARY KEY\n)", "sql"},

		// HTML
		{"html doctype", "<!DOCTYPE html>\n<html><body></body></html>", "html"},
		{"html div", "<div class=\"container\">\n  <p>Hello</p>\n</div>", "html"},

		// CSS
		{"css class", ".container {\n  color: red;\n  margin: 10px;\n}", "css"},
		{"css id", "#header {\n  display: flex;\n  padding: 20px;\n}", "css"},

		// YAML
		{"yaml config", "name: test\nversion: 1.0\nitems:\n  - item1\n  - item2", "yaml"},

		// TOML
		{"toml config", "[package]\nname = \"test\"\nversion = \"1.0\"", "toml"},

		// Fallback
		{"plain text", "This is just some plain text.", "text"},
		{"unknown", "some random content here", "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectLanguage(tt.code)
			if result != tt.expected {
				t.Errorf("detectLanguage() = %q, want %q\ncode: %s", result, tt.expected, tt.code)
			}
		})
	}
}

func TestFormatMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "add language to empty fence",
			input:    "# Title\n\n```\ndef hello():\n    print('hi')\n```\n",
			expected: "# Title\n\n```python\ndef hello():\n    print('hi')\n```\n",
		},
		{
			name:     "preserve existing language",
			input:    "```javascript\nconst x = 1;\n```\n",
			expected: "```javascript\nconst x = 1;\n```\n",
		},
		{
			name:     "fix excessive blank lines",
			input:    "# Title\n\n\n\n\nParagraph\n",
			expected: "# Title\n\nParagraph\n",
		},
		{
			name:     "add json language",
			input:    "```\n{\"key\": \"value\"}\n```\n",
			expected: "```json\n{\"key\": \"value\"}\n```\n",
		},
		{
			name:     "add go language",
			input:    "```\npackage main\n\nfunc main() {\n}\n```\n",
			expected: "```go\npackage main\n\nfunc main() {\n}\n```\n",
		},
		{
			name:     "add bash language",
			input:    "```\n#!/bin/bash\necho hello\n```\n",
			expected: "```bash\n#!/bin/bash\necho hello\n```\n",
		},
		{
			name:     "multiple fences",
			input:    "```\ndef foo():\n    pass\n```\n\nText\n\n```\nconst x = 1;\n```\n",
			expected: "```python\ndef foo():\n    pass\n```\n\nText\n\n```javascript\nconst x = 1;\n```\n",
		},
		{
			name:     "indented fence",
			input:    "   ```\n   SELECT * FROM users\n   ```\n",
			expected: "   ```sql\n   SELECT * FROM users\n   ```\n",
		},
		{
			name:     "no changes needed",
			input:    "# Title\n\nSome text.\n",
			expected: "# Title\n\nSome text.\n",
		},
		{
			name:     "ensure trailing newline",
			input:    "# Title",
			expected: "# Title\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("formatMarkdown() mismatch\ngot:\n%q\nwant:\n%q", result, tt.expected)
			}
		})
	}
}

func BenchmarkDetectLanguage(b *testing.B) {
	samples := []string{
		`{"key": "value", "num": 123}`,
		"def hello():\n    print('hello')",
		"package main\n\nfunc main() {}",
		"const x = () => console.log('hi');",
		"SELECT * FROM users WHERE id = 1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sample := samples[i%len(samples)]
		detectLanguage(sample)
	}
}

func BenchmarkFormatMarkdown(b *testing.B) {
	content := `# Test Document

` + "```" + `
def hello():
    print('hello')
` + "```" + `

Some text here.


` + "```" + `
const x = 1;
console.log(x);
` + "```" + `

More text.
`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatMarkdown(content)
	}
}
