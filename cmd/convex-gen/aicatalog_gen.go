package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AICatalogGenerator emits the AI tool catalog artifact (catalog.ts + index.ts).
type AICatalogGenerator struct {
	config    *Config
	outputDir string
}

// NewAICatalogGenerator constructs an AICatalogGenerator bound to the configured output dir.
func NewAICatalogGenerator(config *Config) *AICatalogGenerator {
	return &AICatalogGenerator{config: config, outputDir: config.GetAICatalogOutputDir()}
}

// catalogFnPath returns the slash-form path (Namespace/Name) used for deny matching + display.
func catalogFnPath(fn ConvexFunction) string {
	if fn.Namespace == "" {
		return fn.Name
	}
	return fn.Namespace + "/" + fn.Name
}

// catalogAPIPath returns the dotted runtime path (api.<apiPath>) for the function.
func catalogAPIPath(fn ConvexFunction) string {
	ns := strings.ReplaceAll(fn.Namespace, "/", ".")
	if ns == "" {
		return fn.Name
	}
	return ns + "." + fn.Name
}

// classifyKind classifies a function as "read" or "write". Queries and forceRead-matched
// actions/mutations are reads; everything else is a write.
func (g *AICatalogGenerator) classifyKind(fnPath string, fn ConvexFunction, forceRead []string) string {
	if fn.Type == FunctionTypeQuery {
		return "read"
	}
	if matchesAnyGlob(fnPath, forceRead) {
		return "read"
	}
	return "write"
}

type catalogEntry struct {
	fnPath, apiPath, kind, description, inputSchema, domain string
}

// Generate writes catalog.ts + index.ts for the given (public-only) functions.
func (g *AICatalogGenerator) Generate(functions []ConvexFunction) error {
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", g.outputDir, err)
	}

	entries := make([]catalogEntry, 0, len(functions))
	for _, fn := range functions {
		fnPath := catalogFnPath(fn)
		if matchesAnyGlob(fnPath, g.config.AI.Deny) {
			continue
		}
		desc := g.config.AI.Descriptions[fnPath]
		if desc == "" {
			desc = fallbackDescription(fnPath, fn)
		}
		domain := fn.Namespace
		if i := strings.Index(domain, "/"); i >= 0 {
			domain = domain[:i]
		}
		entries = append(entries, catalogEntry{
			fnPath:      fnPath,
			apiPath:     catalogAPIPath(fn),
			kind:        g.classifyKind(fnPath, fn, g.config.AI.ForceRead),
			description: desc,
			inputSchema: argsToJSONSchema(fn),
			domain:      domain,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].fnPath < entries[j].fnPath })

	content := g.render(entries)
	if err := os.WriteFile(filepath.Join(g.outputDir, "catalog.ts"), []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write catalog.ts: %w", err)
	}
	return os.WriteFile(filepath.Join(g.outputDir, "index.ts"),
		[]byte("/** Auto-generated. DO NOT EDIT. Run 'convex-gen'. */\nexport * from './catalog';\n"), 0644)
}

func (g *AICatalogGenerator) render(entries []catalogEntry) string {
	var sb strings.Builder
	sb.WriteString("/**\n * Auto-generated AI Tool Catalog\n *\n * DO NOT EDIT MANUALLY - Run 'convex-gen' to regenerate\n *\n")
	sb.WriteString(" * One entry per public Convex function eligible for AI tool-calling.\n */\n\n")
	sb.WriteString("export interface AiToolCatalogEntry {\n")
	sb.WriteString("  fnPath: string;\n  apiPath: string;\n  kind: \"read\" | \"write\";\n")
	sb.WriteString("  description: string;\n  inputSchema: string;\n  domain: string;\n}\n\n")
	sb.WriteString("export const AI_TOOL_CATALOG: AiToolCatalogEntry[] = [\n")
	for _, e := range entries {
		sb.WriteString("  {\n")
		fmt.Fprintf(&sb, "    fnPath: %s,\n", tsString(e.fnPath))
		fmt.Fprintf(&sb, "    apiPath: %s,\n", tsString(e.apiPath))
		fmt.Fprintf(&sb, "    kind: %s,\n", tsString(e.kind))
		fmt.Fprintf(&sb, "    description: %s,\n", tsString(e.description))
		fmt.Fprintf(&sb, "    inputSchema: %s,\n", tsString(e.inputSchema))
		fmt.Fprintf(&sb, "    domain: %s,\n", tsString(e.domain))
		sb.WriteString("  },\n")
	}
	sb.WriteString("];\n")
	return sb.String()
}

// tsString returns a safely-quoted TS string literal (JSON encoding is valid TS).
func tsString(s string) string {
	b, _ := jsonMarshalString(s)
	return b
}
