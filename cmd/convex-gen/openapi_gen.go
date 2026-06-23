package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// OpenAPIGenerator emits an OpenAPI 3.1 spec describing a project's public HTTP
// API. It is self-contained: it scans the Convex tree for `*Api.ts` modules and
// reads the exported `<resource>ApiInput` / `<resource>ApiPatch` /
// `<resource>ApiOutput` validators from each, independent of the public-function
// parser (which skips the internal `*ForApi` functions these modules contain).
type OpenAPIGenerator struct {
	config *Config
}

func NewOpenAPIGenerator(config *Config) *OpenAPIGenerator {
	return &OpenAPIGenerator{config: config}
}

// apiResource is one CRUD resource discovered from a `*Api.ts` module.
type apiResource struct {
	pathSegment string      // URL collection segment, e.g. "vehicles" (from the filename)
	schemaBase  string      // PascalCase schema/operation base, e.g. "Vehicle" (from the validator prefix)
	input       []FieldInfo // <prefix>ApiInput → request body for create
	patch       []FieldInfo // <prefix>ApiPatch → request body for update
	output      []FieldInfo // <prefix>ApiOutput → response shape
}

// matches `export const <prefix>Api(Input|Patch|Output) =`
var apiValidatorRe = regexp.MustCompile(`export\s+const\s+(\w+?)Api(Input|Patch|Output)\s*=`)

// Generate writes the spec and returns the number of resources found.
func (g *OpenAPIGenerator) Generate() (int, error) {
	resources, err := g.discoverResources()
	if err != nil {
		return 0, err
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].pathSegment < resources[j].pathSegment
	})

	spec := g.renderSpec(resources)

	outPath := g.config.GetOpenAPISpecPath()
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, fmt.Errorf("creating output dir: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(spec), 0o644); err != nil {
		return 0, fmt.Errorf("writing spec: %w", err)
	}
	return len(resources), nil
}

func (g *OpenAPIGenerator) discoverResources() ([]apiResource, error) {
	skipDir := map[string]bool{}
	for _, d := range g.config.Skip.Directories {
		skipDir[d] = true
	}

	var resources []apiResource
	err := filepath.Walk(g.config.Convex.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDir[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if !strings.HasSuffix(name, "Api.ts") {
			return nil
		}
		if strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
			return nil
		}
		res, ok := g.parseModule(path, name)
		if ok {
			resources = append(resources, res)
		}
		return nil
	})
	return resources, err
}

// parseModule extracts a resource from one `*Api.ts` file. It needs at least an
// Input and an Output validator with a shared prefix to be considered a resource.
func (g *OpenAPIGenerator) parseModule(path, name string) (apiResource, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return apiResource{}, false
	}
	text := string(content)

	byKind := map[string][]FieldInfo{} // "Input" / "Patch" / "Output" -> fields
	var prefix string
	for _, m := range apiValidatorRe.FindAllStringSubmatchIndex(text, -1) {
		p := text[m[2]:m[3]]
		kind := text[m[4]:m[5]]
		if prefix == "" {
			prefix = p
		}
		body := extractBraceBody(text, m[1])
		if body == "" {
			continue
		}
		byKind[kind] = parseTableFields(body)
	}

	if prefix == "" || len(byKind["Input"]) == 0 || len(byKind["Output"]) == 0 {
		return apiResource{}, false
	}

	return apiResource{
		pathSegment: strings.TrimSuffix(name, "Api.ts"),
		schemaBase:  toPascalCase(prefix),
		input:       byKind["Input"],
		patch:       byKind["Patch"],
		output:      byKind["Output"],
	}, true
}

// extractBraceBody returns the contents of the first balanced `{ ... }` at or
// after fromIdx (handles `v.object({...})` and plain `{...}` validator forms).
func extractBraceBody(s string, fromIdx int) string {
	open := strings.IndexByte(s[fromIdx:], '{')
	if open < 0 {
		return ""
	}
	open += fromIdx
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[open+1 : i]
			}
		}
	}
	return ""
}
