# Markdown Formatter

**Repository:** [claude-hooks](https://github.com/milehighideas/claude-hooks) (`cmd/markdown-formatter`)

A Claude Code PostToolUse hook that automatically formats Markdown files by adding language tags to unlabeled code fences and fixing excessive blank lines.

## Overview

The markdown-formatter tool enhances Markdown documentation by:

1. **Detecting and adding language tags** to code fences that lack them
2. **Fixing excessive blank lines** (3+ consecutive newlines reduced to 2)
3. **Ensuring proper file formatting** (trailing newline)

This tool is designed to run automatically as a Claude Code PostToolUse hook after files are written, maintaining consistent markdown formatting without manual intervention.

## Usage

### As a Claude Code Hook

The primary usage is as a PostToolUse hook in Claude Code configuration. The tool processes the JSON input from Claude Code and automatically formats any `.md` or `.mdx` files that are written.

**Input Format:**

```json
{
  "tool": "Write",
  "tool_input": {
    "file_path": "/path/to/document.md"
  }
}
```

### Command Line

You can also run the formatter directly from the command line:

```bash
echo '{"tool":"Write","tool_input":{"file_path":"./my-file.md"}}' | ./markdown-formatter
```

The tool reads JSON from stdin, processes the specified file, and outputs status to stdout.

## Supported Language Detection

The tool detects and tags the following languages:

- **JSON** - JSON objects and arrays (validated by JSON parser)
- **Python** - `def`, `import`, `from` statements
- **Go** - `package`, `func`, `import` declarations
- **Rust** - `fn`, `impl`, `let mut` patterns
- **TypeScript** - `interface`, `type`, generics (`<T>`)
- **JavaScript** - `function`, `const`, arrow functions (`=>`), `console` calls
- **JSX/TSX** - React components with capital-case tags and `className`
- **Bash** - Shebang (`#!/bin/bash`), control structures, shell commands
- **SQL** - Standard SQL keywords (`SELECT`, `INSERT`, `UPDATE`, etc.)
- **HTML** - HTML tags and DOCTYPE declarations
- **CSS** - Class and ID selectors with property declarations
- **YAML** - Key-value pairs and list items
- **TOML** - Section headers (`[section]`) and key assignments
- **Text** - Fallback for unrecognized code

## Command Line Arguments

The tool does not accept command-line arguments directly. Instead, it reads a JSON structure from stdin with the following field:

- `tool_input.file_path` - **Required** - Path to the markdown file to format

## Environment Variables

No environment variables are required or used by this tool.

## Exit Codes

- **0** - Success (file formatted or no changes needed), or file was skipped (not .md/.mdx, doesn't exist, parse error)
- **1** - Error reading or writing the file

## Behavior

### File Processing

1. Reads JSON input from stdin
2. Skips processing if:
   - File path is empty
   - File is not `.md` or `.mdx` extension
   - File does not exist
   - JSON cannot be parsed (gracefully ignores)
3. Processes code fences:
   - Detects language in unlabeled fences
   - Preserves existing language tags
   - Maintains fence indentation
4. Fixes formatting:
   - Reduces 3+ blank lines to 2
   - Ensures single trailing newline
5. Writes changes only if content differs
6. Outputs status message to stdout

### Code Fence Parsing

Code fences are identified by triple backticks (` ``` `) with optional indentation (0-3 spaces). The parser:

- Extracts existing language tag if present
- Analyzes fence body content for language detection
- Preserves indentation in the fence markers
- Handles nested or indented code blocks

## Examples

### Basic Usage with JSON Input

```bash
echo '{"tool":"Write","tool_input":{"file_path":"./docs/api.md"}}' | ./markdown-formatter
# Output: Fixed markdown formatting in ./docs/api.md
```

### File with Unlabeled Python Fence

Before:

```text
# Python Guide

```

def hello():
print('hello')

```text

```

After:

````python
# Python Guide

```python
def hello():
    print('hello')
````

```text

### Fixing Excessive Blank Lines

Before:
```

# Title

Paragraph

```text

After:
```

# Title

Paragraph

```text

### Multiple Code Fences with Mixed Languages

Before:
```

# API Documentation

```python
import json
data = {"key": "value"}
```

Response format:

```json
{
  "status": "success",
  "data": {}
}
```

```text

After:
```

# API Documentation

```python
import json
data = {"key": "value"}
```

Response format:

```json
{
  "status": "success",
  "data": {}
}
```

```text

### Preserving Existing Language Tags

The tool never overwrites explicit language tags:

```

```go
func main() {
    // This stays as golang
}
```

````bash

Remains unchanged as `go`.

## Implementation Details

The tool uses regular expressions to detect language patterns:

- **JSON** - Attempts to parse content as JSON for reliable detection
- **Go** - Looks for `package`, `func`, or `import` statements
- **Python** - Matches `def` or import statements
- **TypeScript** - Detects `interface`, `type`, or generic syntax
- **JavaScript** - Finds function declarations, const assignments, or console calls

Detection is performed in a specific order to handle overlapping patterns (e.g., TypeScript before JavaScript, Go before other C-like languages).

## Performance

The tool is optimized for single-file processing and includes benchmarks:

- Language detection handles typical code blocks efficiently
- Regex patterns are compiled as package-level variables
- File I/O is minimized (read once, write only if changed)

## Testing

Comprehensive test coverage includes:

- 16+ language detection scenarios
- 10+ markdown formatting cases
- Edge cases (indented fences, multiple fences, excessive blank lines)
- Benchmark tests for performance validation

Run tests with:
```bash
go test ./markdown-formatter
````

## Notes

- The tool processes files in-place; no backup is created
- Indented code fences (0-3 spaces) are supported per Markdown spec
- File permissions are preserved (0644)
- The tool gracefully ignores parse errors in JSON input
