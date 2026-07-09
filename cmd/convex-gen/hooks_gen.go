package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// HooksGenerator generates React hooks for Convex functions
type HooksGenerator struct {
	config       *Config
	outputDir    string
	queriesDir   string
	mutationsDir string
	actionsDir   string
}

// NewHooksGenerator creates a hooks generator
func NewHooksGenerator(config *Config) *HooksGenerator {
	outputDir := config.GetHooksOutputDir()
	return &HooksGenerator{
		config:       config,
		outputDir:    outputDir,
		queriesDir:   filepath.Join(outputDir, "queries"),
		mutationsDir: filepath.Join(outputDir, "mutations"),
		actionsDir:   filepath.Join(outputDir, "actions"),
	}
}

// FunctionGroup represents functions grouped by sub-namespace within a top-level namespace
type FunctionGroup struct {
	SubNamespace string
	Functions    []ConvexFunction
}

// Generate creates all hook files
func (g *HooksGenerator) Generate(functions []ConvexFunction) error {
	// Create output directories
	for _, dir := range []string{g.queriesDir, g.mutationsDir, g.actionsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Clean existing files
	for _, dir := range []string{g.queriesDir, g.mutationsDir, g.actionsDir} {
		if err := cleanDirectory(dir); err != nil {
			return err
		}
	}

	// Group functions by type and TOP-LEVEL namespace
	queries := make(map[string][]ConvexFunction)
	mutations := make(map[string][]ConvexFunction)
	actions := make(map[string][]ConvexFunction)

	for _, fn := range functions {
		// Extract top-level namespace (e.g., "events" from "events/adminCheckIn")
		topLevel := getTopLevelNamespace(fn.Namespace)

		switch fn.Type {
		case FunctionTypeQuery:
			queries[topLevel] = append(queries[topLevel], fn)
		case FunctionTypeMutation:
			mutations[topLevel] = append(mutations[topLevel], fn)
		case FunctionTypeAction:
			actions[topLevel] = append(actions[topLevel], fn)
		}
	}

	// Generate query hooks
	queryFiles, err := g.generateHookFiles(queries, g.queriesDir, "query")
	if err != nil {
		return err
	}

	// Generate mutation hooks
	mutationFiles, err := g.generateHookFiles(mutations, g.mutationsDir, "mutation")
	if err != nil {
		return err
	}

	// Generate action hooks
	actionFiles, err := g.generateHookFiles(actions, g.actionsDir, "action")
	if err != nil {
		return err
	}

	// Generate index files
	if err := generateIndexFile(g.queriesDir, queryFiles); err != nil {
		return err
	}
	if err := generateIndexFile(g.mutationsDir, mutationFiles); err != nil {
		return err
	}
	if err := generateIndexFile(g.actionsDir, actionFiles); err != nil {
		return err
	}

	return nil
}

// getTopLevelNamespace extracts the top-level namespace from a full namespace path
func getTopLevelNamespace(namespace string) string {
	parts := splitNamespace(namespace)
	return parts[0]
}

// getSubNamespace extracts the sub-namespace from a full namespace path
func getSubNamespace(namespace string) string {
	parts := splitNamespace(namespace)
	if len(parts) > 1 {
		return strings.Join(parts[1:], "/")
	}
	return ""
}

// generateHookFiles creates hook files based on fileStructure config
func (g *HooksGenerator) generateHookFiles(byNamespace map[string][]ConvexFunction, outputDir string, funcType string) ([]string, error) {
	fileStructure := g.config.DataLayer.FileStructure
	var files []string

	// Generate grouped files (one per top-level namespace)
	if fileStructure == "grouped" || fileStructure == "both" {
		for topNamespace, funcs := range byNamespace {
			fileName := "use" + capitalize(topNamespace)
			filePath := filepath.Join(outputDir, fileName+".ts")

			content := g.generateGroupedHookFileContent(topNamespace, funcs, funcType)

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", filePath, err)
			}

			files = append(files, fileName)
		}
	}

	// Generate split files (one per sub-namespace)
	if fileStructure == "split" || fileStructure == "both" {
		for topNamespace, funcs := range byNamespace {
			// Group by full namespace (sub-namespace)
			byFullNamespace := make(map[string][]ConvexFunction)
			for _, fn := range funcs {
				byFullNamespace[fn.Namespace] = append(byFullNamespace[fn.Namespace], fn)
			}

			for fullNamespace, subFuncs := range byFullNamespace {
				fileName := namespaceToFileName(fullNamespace)
				filePath := filepath.Join(outputDir, fileName+".ts")

				content := g.generateSplitHookFileContent(topNamespace, fullNamespace, subFuncs, funcType)

				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					return nil, fmt.Errorf("failed to write %s: %w", filePath, err)
				}

				files = append(files, fileName)
			}
		}
	}

	sort.Strings(files)
	// Remove duplicates (in case both modes generate same file name)
	files = uniqueStrings(files)
	return files, nil
}

// uniqueStrings removes duplicates from a sorted slice
func uniqueStrings(s []string) []string {
	if len(s) == 0 {
		return s
	}
	result := []string{s[0]}
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			result = append(result, s[i])
		}
	}
	return result
}

// generateSplitHookFileContent creates content for a single sub-namespace file
func (g *HooksGenerator) generateSplitHookFileContent(topNamespace, fullNamespace string, funcs []ConvexFunction, funcType string) string {
	var sb strings.Builder

	// Header
	sb.WriteString("/**\n")
	fmt.Fprintf(&sb, " * AUTO-GENERATED %s HOOKS - DO NOT EDIT\n", strings.ToUpper(funcType))
	fmt.Fprintf(&sb, " * Namespace: %s\n", fullNamespace)
	sb.WriteString(" *\n")
	sb.WriteString(" * Run 'convex-gen' to regenerate this file.\n")
	sb.WriteString(" */\n\n")

	// Determine imports needed (only queries use Id and FunctionArgs types)
	needsPagination := false
	needsRegularQuery := false
	needsId := false
	needsFunctionArgs := false

	// Only check for Id/FunctionArgs in queries - mutations/actions don't use typed args
	if funcType == "query" {
		for _, fn := range funcs {
			if fn.IsPaginated {
				needsPagination = true
			} else {
				needsRegularQuery = true
			}
			if fn.UseFunctionArgs {
				needsFunctionArgs = true
			} else {
				// Only check for Id if NOT using FunctionArgs (otherwise Id is embedded in FunctionArgs)
				for _, arg := range fn.Args {
					if arg.IsID {
						needsId = true
					}
				}
			}
		}
	}

	// Imports
	switch funcType {
	case "query":
		if needsPagination && needsRegularQuery {
			sb.WriteString("import { useQuery, usePaginatedQuery } from 'convex/react';\n")
		} else if needsPagination {
			sb.WriteString("import { usePaginatedQuery } from 'convex/react';\n")
		} else {
			sb.WriteString("import { useQuery } from 'convex/react';\n")
		}
	case "mutation":
		sb.WriteString("import { useMutation } from 'convex/react';\n")
	case "action":
		sb.WriteString("import { useAction } from 'convex/react';\n")
	}

	fmt.Fprintf(&sb, "import { api } from '%s';\n", g.config.Imports.API)

	if needsId {
		fmt.Fprintf(&sb, "import type { Id } from '%s';\n", g.config.Imports.DataModel)
	}
	if needsFunctionArgs {
		sb.WriteString("import type { FunctionArgs } from 'convex/server';\n")
	}
	if g.config.DataLayer.TypedReturns && funcType == "query" && needsRegularQuery {
		sb.WriteString("import type { FunctionReturnType } from 'convex/server';\n")
	}
	if g.config.DataLayer.TypedArgs && funcType == "mutation" {
		sb.WriteString("import type { ReactMutation } from 'convex/react';\n")
	}
	if g.config.DataLayer.TypedArgs && funcType == "action" {
		sb.WriteString("import type { ReactAction } from 'convex/react';\n")
	}

	sb.WriteString("\n")

	// For split files, always include sub-namespace in hook names to avoid
	// collisions when index.ts re-exports from multiple files
	for _, fn := range funcs {
		sb.WriteString(g.generateSplitHook(topNamespace, fn))
	}

	return sb.String()
}

// generateGroupedHookFileContent creates the content for a grouped hooks file
// This groups all functions from sub-namespaces into a single file with section comments
func (g *HooksGenerator) generateGroupedHookFileContent(topNamespace string, funcs []ConvexFunction, funcType string) string {
	var sb strings.Builder

	// Group functions by sub-namespace
	groups := make(map[string][]ConvexFunction)
	for _, fn := range funcs {
		subNs := getSubNamespace(fn.Namespace)
		if subNs == "" {
			subNs = topNamespace // Use top-level if no sub-namespace
		}
		groups[subNs] = append(groups[subNs], fn)
	}

	// Detect function name collisions across sub-namespaces
	// Used by "auto" mode to add sub-namespace only when needed
	hookNaming := g.config.DataLayer.HookNaming
	funcNameCount := make(map[string]int)
	for _, fn := range funcs {
		baseName := "use" + capitalize(topNamespace) + capitalize(fn.Name)
		funcNameCount[baseName]++
	}
	collisions := make(map[string]bool)
	for name, count := range funcNameCount {
		if count > 1 {
			collisions[name] = true
		}
	}

	// In "flat" mode, deduplicate: first function seen with a given baseName wins
	seen := make(map[string]bool)

	// Sort sub-namespaces for consistent output
	var subNamespaces []string
	for subNs := range groups {
		subNamespaces = append(subNamespaces, subNs)
	}
	sort.Strings(subNamespaces)

	// Header
	sb.WriteString("/**\n")
	fmt.Fprintf(&sb, " * %s %s Hooks\n", capitalize(topNamespace), capitalize(funcType))
	sb.WriteString(" * Auto-generated React query hooks for Convex functions\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * ⚠️ DO NOT EDIT MANUALLY - Run 'npm run generate:hooks' to regenerate\n")
	sb.WriteString(" *\n")
	sb.WriteString(" * Features:\n")
	sb.WriteString(" * ✅ Typed parameters with null safety\n")
	sb.WriteString(" * ✅ Conditional queries with \"skip\"\n")
	if funcType == "query" {
		sb.WriteString(" * ✅ Paginated queries with usePaginatedQuery\n")
	}
	sb.WriteString(" * ✅ JSDoc documentation\n")
	sb.WriteString(" */\n\n")

	// Determine all imports needed (only queries use Id and FunctionArgs types)
	needsPagination := false
	needsRegularQuery := false
	needsId := false
	needsFunctionArgs := false

	// Only check for Id/FunctionArgs in queries - mutations/actions don't use typed args
	if funcType == "query" {
		for _, fn := range funcs {
			if fn.IsPaginated {
				needsPagination = true
			} else {
				needsRegularQuery = true
			}
			if fn.UseFunctionArgs {
				needsFunctionArgs = true
			} else {
				// Only check for Id if NOT using FunctionArgs (otherwise Id is embedded in FunctionArgs)
				for _, arg := range fn.Args {
					if arg.IsID {
						needsId = true
					}
				}
			}
		}
	}

	// Imports - use configured import paths
	switch funcType {
	case "query":
		if needsPagination && needsRegularQuery {
			sb.WriteString("import { useQuery, usePaginatedQuery } from \"convex/react\";\n")
		} else if needsPagination {
			sb.WriteString("import { usePaginatedQuery } from \"convex/react\";\n")
		} else {
			sb.WriteString("import { useQuery } from \"convex/react\";\n")
		}
	case "mutation":
		sb.WriteString("import { useMutation } from \"convex/react\";\n")
	case "action":
		sb.WriteString("import { useAction } from \"convex/react\";\n")
	}

	// API import - use configured path
	fmt.Fprintf(&sb, "import { api } from \"%s\";\n", g.config.Imports.API)

	if needsId {
		fmt.Fprintf(&sb, "import type { Id } from \"%s\";\n", g.config.Imports.DataModel)
	}
	if needsFunctionArgs {
		sb.WriteString("import type { FunctionArgs } from \"convex/server\";\n")
	}
	if g.config.DataLayer.TypedReturns && funcType == "query" && needsRegularQuery {
		sb.WriteString("import type { FunctionReturnType } from \"convex/server\";\n")
	}
	if g.config.DataLayer.TypedArgs && funcType == "mutation" {
		sb.WriteString("import type { ReactMutation } from \"convex/react\";\n")
	}
	if g.config.DataLayer.TypedArgs && funcType == "action" {
		sb.WriteString("import type { ReactAction } from \"convex/react\";\n")
	}

	// Generate hooks grouped by sub-namespace with section comments
	for _, subNs := range subNamespaces {
		subFuncs := groups[subNs]

		// Section comment - capitalize and uppercase the sub-namespace name
		sectionName := strings.ToUpper(toCamelCase(subNs))
		funcTypeLabel := strings.ToUpper(funcType)
		if funcTypeLabel == "QUERY" {
			funcTypeLabel = "QUERIES"
		} else {
			funcTypeLabel += "S"
		}
		fmt.Fprintf(&sb, "// ============= %s %s =============\n\n", sectionName, funcTypeLabel)

		// Generate hooks for this sub-namespace
		for _, fn := range subFuncs {
			hook, hookName := g.generateHook(topNamespace, fn, collisions, hookNaming)
			if hookNaming == "flat" {
				if seen[hookName] {
					continue // skip duplicate — first-seen wins
				}
				seen[hookName] = true
			}
			sb.WriteString(hook)
		}
	}

	return sb.String()
}

// generateHook creates a single hook function matching TypeScript output format.
// Returns the generated code and the hookName (used by caller for dedup in flat mode).
func (g *HooksGenerator) generateHook(topNamespace string, fn ConvexFunction, collisions map[string]bool, hookNaming string) (string, string) {
	var sb strings.Builder

	// Base hook name without sub-namespace
	baseName := "use" + capitalize(topNamespace) + capitalize(fn.Name)

	// Determine hook name based on naming strategy
	var hookName string
	switch hookNaming {
	case "qualified":
		// Always include sub-namespace
		subNs := getSubNamespace(fn.Namespace)
		if subNs != "" && subNs != topNamespace {
			hookName = "use" + capitalize(topNamespace) + capitalize(toCamelCase(subNs)) + capitalize(fn.Name)
		} else {
			hookName = baseName
		}
	case "flat":
		// Never include sub-namespace
		hookName = baseName
	default: // "auto"
		// Include sub-namespace only on collision
		if collisions[baseName] {
			subNs := getSubNamespace(fn.Namespace)
			if subNs != "" && subNs != topNamespace {
				hookName = "use" + capitalize(topNamespace) + capitalize(toCamelCase(subNs)) + capitalize(fn.Name)
			} else {
				hookName = baseName
			}
		} else {
			hookName = baseName
		}
	}
	apiPath := toApiPath(fn.Namespace, fn.Name)

	// JSDoc
	sb.WriteString("/**\n")
	fmt.Fprintf(&sb, " * Hook to %s\n", toNaturalLanguage(fn.Name))

	hasRequiredSkippable := hasRequiredSkippableArg(fn.Args)
	requireShouldSkip := g.config.DataLayer.RequireAuthGatedShouldSkip && fn.RequiresAuth
	// Mirrors generateParamsV2: when shouldSkip/options is forced required and
	// this query has args to reorder around, the doc block documents required
	// args, then shouldSkip/options, then optional args — matching the
	// reordered signature. Every other case (flag off, non-auth-gated, a
	// required skippable ID arg, or zero args) falls through to the
	// historical single-pass doc loop + trailing shouldSkip/options line,
	// byte-for-byte unchanged.
	reorderDocs := requireShouldSkip && !hasRequiredSkippable && fn.Type == FunctionTypeQuery && !fn.UseFunctionArgs && len(fn.Args) > 0

	writeArgDoc := func(arg ArgInfo) {
		if arg.TableName != "" {
			if arg.Optional {
				fmt.Fprintf(&sb, " * @param %s - ID of %s (optional)\n", arg.Name, arg.TableName)
			} else {
				fmt.Fprintf(&sb, " * @param %s - ID of %s\n", arg.Name, arg.TableName)
			}
		} else {
			if arg.Optional {
				fmt.Fprintf(&sb, " * @param %s - %s value (optional)\n", arg.Name, arg.Type)
			} else {
				fmt.Fprintf(&sb, " * @param %s - %s value\n", arg.Name, arg.Type)
			}
		}
	}

	if len(fn.Args) > 0 && fn.Type == FunctionTypeQuery {
		sb.WriteString(" *\n")
		if reorderDocs {
			for _, arg := range fn.Args {
				if !arg.Optional {
					writeArgDoc(arg)
				}
			}
			if fn.IsPaginated {
				sb.WriteString(" * @param options - Required. This query requires authentication; pass { shouldSkip: true } to skip it for unauthenticated callers.\n")
			} else {
				sb.WriteString(" * @param shouldSkip - Required. This query requires authentication; pass true to skip it for unauthenticated callers.\n")
			}
			for _, arg := range fn.Args {
				if arg.Optional {
					writeArgDoc(arg)
				}
			}
		} else {
			for _, arg := range fn.Args {
				writeArgDoc(arg)
			}
		}
	}

	// Add shouldSkip param in JSDoc for no-required-ID non-paginated queries.
	// (Paginated queries expose shouldSkip via options.shouldSkip — documented
	// below.) Skipped when reorderDocs already emitted it above, in position.
	if !reorderDocs && fn.Type == FunctionTypeQuery && !fn.UseFunctionArgs && !hasRequiredSkippable && !fn.IsPaginated {
		shouldSkipDoc := " * @param shouldSkip - Skip the query if true (e.g., when user not authenticated)\n"
		if requireShouldSkip {
			shouldSkipDoc = " * @param shouldSkip - Required. This query requires authentication; pass true to skip it for unauthenticated callers.\n"
		}
		if len(fn.Args) > 0 {
			sb.WriteString(shouldSkipDoc)
		} else {
			sb.WriteString(" *\n")
			sb.WriteString(shouldSkipDoc)
		}
	}

	if !reorderDocs && fn.IsPaginated {
		switch {
		case requireShouldSkip && !hasRequiredSkippable && !fn.UseFunctionArgs:
			sb.WriteString(" * @param options - Required. This query requires authentication; pass { shouldSkip: true } to skip it for unauthenticated callers.\n")
		case !hasRequiredSkippable && !fn.UseFunctionArgs:
			sb.WriteString(" * @param options - Pagination options (optional). Pass { shouldSkip: true } to skip the query.\n")
		default:
			sb.WriteString(" * @param options - Pagination options (optional)\n")
		}
	}
	sb.WriteString(" */\n")

	// Function signature
	params := g.generateParamsV2(fn)
	// Exactly one of these is non-empty: returnAnnotation for queries
	// (typedReturns), argsAnnotation for mutations/actions (typedArgs). Both are
	// "" when their flag is disabled, preserving the historical untyped output.
	returnAnnotation := g.generateReturnAnnotation(fn, apiPath)
	argsAnnotation := g.generateArgsAnnotation(fn, apiPath)
	fmt.Fprintf(&sb, "export function %s(%s)%s%s {\n", hookName, params, returnAnnotation, argsAnnotation)

	// Add @ts-ignore comment for deep type issues
	sb.WriteString("  // @ts-ignore - TS2589: Deep type instantiation with nested API path\n")

	// Function body
	sb.WriteString(g.generateHookBodyV2(fn, apiPath))
	sb.WriteString("}\n\n")

	return sb.String(), hookName
}

// generateSplitHook creates a hook for split files - always includes sub-namespace in name
func (g *HooksGenerator) generateSplitHook(topNamespace string, fn ConvexFunction) string {
	var sb strings.Builder

	// For split files, always include sub-namespace to ensure unique names across files
	subNs := getSubNamespace(fn.Namespace)
	var hookName string
	if subNs != "" && subNs != topNamespace {
		hookName = "use" + capitalize(topNamespace) + capitalize(toCamelCase(subNs)) + capitalize(fn.Name)
	} else {
		hookName = "use" + capitalize(topNamespace) + capitalize(fn.Name)
	}
	apiPath := toApiPath(fn.Namespace, fn.Name)

	// JSDoc
	sb.WriteString("/**\n")
	fmt.Fprintf(&sb, " * Hook to %s\n", toNaturalLanguage(fn.Name))

	hasRequiredSkippable := hasRequiredSkippableArg(fn.Args)
	requireShouldSkip := g.config.DataLayer.RequireAuthGatedShouldSkip && fn.RequiresAuth
	// Mirrors generateParamsV2: when shouldSkip/options is forced required and
	// this query has args to reorder around, the doc block documents required
	// args, then shouldSkip/options, then optional args — matching the
	// reordered signature. Every other case (flag off, non-auth-gated, a
	// required skippable ID arg, or zero args) falls through to the
	// historical single-pass doc loop + trailing shouldSkip/options line,
	// byte-for-byte unchanged.
	reorderDocs := requireShouldSkip && !hasRequiredSkippable && fn.Type == FunctionTypeQuery && !fn.UseFunctionArgs && len(fn.Args) > 0

	writeArgDoc := func(arg ArgInfo) {
		if arg.TableName != "" {
			if arg.Optional {
				fmt.Fprintf(&sb, " * @param %s - ID of %s (optional)\n", arg.Name, arg.TableName)
			} else {
				fmt.Fprintf(&sb, " * @param %s - ID of %s\n", arg.Name, arg.TableName)
			}
		} else {
			if arg.Optional {
				fmt.Fprintf(&sb, " * @param %s - %s value (optional)\n", arg.Name, arg.Type)
			} else {
				fmt.Fprintf(&sb, " * @param %s - %s value\n", arg.Name, arg.Type)
			}
		}
	}

	if len(fn.Args) > 0 && fn.Type == FunctionTypeQuery {
		sb.WriteString(" *\n")
		if reorderDocs {
			for _, arg := range fn.Args {
				if !arg.Optional {
					writeArgDoc(arg)
				}
			}
			if fn.IsPaginated {
				sb.WriteString(" * @param options - Required. This query requires authentication; pass { shouldSkip: true } to skip it for unauthenticated callers.\n")
			} else {
				sb.WriteString(" * @param shouldSkip - Required. This query requires authentication; pass true to skip it for unauthenticated callers.\n")
			}
			for _, arg := range fn.Args {
				if arg.Optional {
					writeArgDoc(arg)
				}
			}
		} else {
			for _, arg := range fn.Args {
				writeArgDoc(arg)
			}
		}
	}

	// Add shouldSkip param in JSDoc for no-required-ID non-paginated queries.
	// (Paginated queries expose shouldSkip via options.shouldSkip — documented
	// below.) Skipped when reorderDocs already emitted it above, in position.
	if !reorderDocs && fn.Type == FunctionTypeQuery && !fn.UseFunctionArgs && !hasRequiredSkippable && !fn.IsPaginated {
		shouldSkipDoc := " * @param shouldSkip - Skip the query if true (e.g., when user not authenticated)\n"
		if requireShouldSkip {
			shouldSkipDoc = " * @param shouldSkip - Required. This query requires authentication; pass true to skip it for unauthenticated callers.\n"
		}
		if len(fn.Args) > 0 {
			sb.WriteString(shouldSkipDoc)
		} else {
			sb.WriteString(" *\n")
			sb.WriteString(shouldSkipDoc)
		}
	}

	if !reorderDocs && fn.IsPaginated {
		switch {
		case requireShouldSkip && !hasRequiredSkippable && !fn.UseFunctionArgs:
			sb.WriteString(" * @param options - Required. This query requires authentication; pass { shouldSkip: true } to skip it for unauthenticated callers.\n")
		case !hasRequiredSkippable && !fn.UseFunctionArgs:
			sb.WriteString(" * @param options - Pagination options (optional). Pass { shouldSkip: true } to skip the query.\n")
		default:
			sb.WriteString(" * @param options - Pagination options (optional)\n")
		}
	}
	sb.WriteString(" */\n")

	// Function signature
	params := g.generateParamsV2(fn)
	// Exactly one of these is non-empty: returnAnnotation for queries
	// (typedReturns), argsAnnotation for mutations/actions (typedArgs). Both are
	// "" when their flag is disabled, preserving the historical untyped output.
	returnAnnotation := g.generateReturnAnnotation(fn, apiPath)
	argsAnnotation := g.generateArgsAnnotation(fn, apiPath)
	fmt.Fprintf(&sb, "export function %s(%s)%s%s {\n", hookName, params, returnAnnotation, argsAnnotation)

	// Add @ts-ignore comment for deep type issues
	sb.WriteString("  // @ts-ignore - TS2589: Deep type instantiation with nested API path\n")

	// Function body
	sb.WriteString(g.generateHookBodyV2(fn, apiPath))
	sb.WriteString("}\n\n")

	return sb.String()
}

// hasRequiredSkippableArg checks if there are required ID/string params that drive skip logic
func hasRequiredSkippableArg(args []ArgInfo) bool {
	for _, arg := range args {
		if arg.IsID && !arg.Optional {
			return true
		}
	}
	return false
}

// generateParamsV2 creates the parameter list matching TypeScript output
func (g *HooksGenerator) generateParamsV2(fn ConvexFunction) string {
	if fn.Type != FunctionTypeQuery {
		return ""
	}

	if fn.UseFunctionArgs {
		apiPath := toApiPath(fn.Namespace, fn.Name)
		var params string
		if fn.IsPaginated {
			// Paginated queries: omit `paginationOpts` from caller args because the hook
			// body provides it via the `usePaginatedQuery` third argument. Without Omit,
			// callers would have to pass `paginationOpts` explicitly even though the hook
			// supplies it internally.
			params = fmt.Sprintf("args: Omit<FunctionArgs<typeof %s>, 'paginationOpts'> | null", apiPath)
			params += ", options?: { initialNumItems?: number }"
		} else {
			params = fmt.Sprintf("args: FunctionArgs<typeof %s> | null", apiPath)
		}
		return params
	}

	var params []string

	// Required params first (TypeScript requirement: required params cannot
	// follow optional ones).
	for _, arg := range fn.Args {
		if !arg.Optional {
			paramType := g.getTypeScriptType(arg)
			params = append(params, fmt.Sprintf("%s: %s", arg.Name, paramType))
		}
	}

	// When RequireAuthGatedShouldSkip is enabled and this query's handler
	// calls a configured auth helper, shouldSkip/options is REQUIRED instead
	// of optional — forces every caller to reckon with the unauthenticated
	// case at compile time. Defaults to the historical optional behavior
	// otherwise (flag off, non-auth-gated, or a required skippable ID arg
	// already exists — that arg drives skip logic instead).
	//
	// A required shouldSkip/options must be inserted here, right after the
	// required args and before the optional args below: TypeScript forbids a
	// required param after an optional one. This reorders the signature
	// relative to the historical trailing position for any query that also
	// has optional args (e.g. `roleId?: string`) — a deliberate, call-site-
	// breaking change once the flag is on for such a query.
	hasRequiredSkippable := hasRequiredSkippableArg(fn.Args)
	requireShouldSkip := g.config.DataLayer.RequireAuthGatedShouldSkip && fn.RequiresAuth
	reorderShouldSkip := requireShouldSkip && !hasRequiredSkippable
	if reorderShouldSkip {
		if fn.IsPaginated {
			params = append(params, "options: { initialNumItems?: number; shouldSkip: boolean }")
		} else {
			params = append(params, "shouldSkip: boolean")
		}
	}

	for _, arg := range fn.Args {
		if arg.Optional {
			paramType := g.getTypeScriptType(arg)
			params = append(params, fmt.Sprintf("%s?: %s", arg.Name, paramType))
		}
	}

	// Historical trailing position: only reached when shouldSkip/options
	// wasn't already inserted above (i.e. it stays optional, or there's a
	// required skippable ID arg driving skip logic instead).
	if !reorderShouldSkip {
		if !hasRequiredSkippable && !fn.IsPaginated {
			params = append(params, "shouldSkip?: boolean")
		}
		if fn.IsPaginated {
			if !hasRequiredSkippable {
				params = append(params, "options?: { initialNumItems?: number; shouldSkip?: boolean }")
			} else {
				params = append(params, "options?: { initialNumItems?: number }")
			}
		}
	}

	return strings.Join(params, ", ")
}

// getTypeScriptType returns the TypeScript type for an argument
func (g *HooksGenerator) getTypeScriptType(arg ArgInfo) string {
	if arg.IsID {
		baseType := fmt.Sprintf("Id<\"%s\">", arg.TableName)
		// Add array suffix for array ID types (e.g., v.array(v.id("projects")))
		if arg.IsArrayID {
			baseType += "[]"
		}
		if arg.Optional {
			return baseType + " | null"
		}
		return baseType + " | null | undefined"
	}

	baseType := arg.Type
	if arg.Optional {
		return baseType + " | null"
	}
	return baseType
}

// generateReturnAnnotation produces the TypeScript return-type annotation appended
// to a hook's function signature when `dataLayer.typedReturns` is enabled.
//
// Returns "" (no annotation) when:
//   - typedReturns is disabled in config (preserves existing untyped behavior)
//   - the function is not a query (mutation/action arg typing is handled by
//     generateArgsAnnotation under the separate typedArgs flag)
//   - the query is paginated (the existing emit already preserves return types via usePaginatedQuery's generic)
//
// Otherwise returns ": FunctionReturnType<typeof <apiPath>> | undefined". The trailing
// `| undefined` matches useQuery's runtime contract — undefined while loading or when args === "skip".
func (g *HooksGenerator) generateReturnAnnotation(fn ConvexFunction, apiPath string) string {
	if !g.config.DataLayer.TypedReturns {
		return ""
	}
	if fn.Type != FunctionTypeQuery {
		return ""
	}
	if fn.IsPaginated {
		return ""
	}
	return fmt.Sprintf(": FunctionReturnType<typeof %s> | undefined", apiPath)
}

// generateArgsAnnotation produces the TypeScript return-type annotation for a
// mutation/action hook when `dataLayer.typedArgs` is enabled.
//
// The hook body is emitted under a `// @ts-ignore - TS2589` (the nested api
// path overflows TypeScript's instantiation depth during inference). That
// suppression also erases the inferred call signature, so without an explicit
// annotation `useFooMutation()` returns an untyped function and caller
// arguments are never checked against the validator. Hoisting the type onto
// the signature — exactly as generateReturnAnnotation does for queries —
// restores arg checking while keeping the body suppressed.
//
// Returns "" (no annotation, the historical behavior) when:
//   - typedArgs is disabled in config (this is the default — backwards compatible)
//   - the function is a query (queries are covered by typedReturns instead)
func (g *HooksGenerator) generateArgsAnnotation(fn ConvexFunction, apiPath string) string {
	if !g.config.DataLayer.TypedArgs {
		return ""
	}
	switch fn.Type {
	case FunctionTypeMutation:
		return fmt.Sprintf(": ReactMutation<typeof %s>", apiPath)
	case FunctionTypeAction:
		return fmt.Sprintf(": ReactAction<typeof %s>", apiPath)
	default:
		return ""
	}
}

// generateHookBodyV2 creates the body of a hook function matching TypeScript output
func (g *HooksGenerator) generateHookBodyV2(fn ConvexFunction, apiPath string) string {
	var sb strings.Builder

	switch fn.Type {
	case FunctionTypeQuery:
		if fn.UseFunctionArgs {
			if fn.IsPaginated {
				sb.WriteString("  return usePaginatedQuery(\n")
				fmt.Fprintf(&sb, "    %s,\n", apiPath)
				sb.WriteString("    args ?? \"skip\",\n")
				sb.WriteString("    { initialNumItems: options?.initialNumItems || 20 }\n")
				sb.WriteString("  );\n")
			} else {
				fmt.Fprintf(&sb, "  return useQuery(%s, args ?? \"skip\");\n", apiPath)
			}
		} else if fn.IsPaginated {
			hasRequiredSkippable := hasRequiredSkippableArg(fn.Args)
			sb.WriteString("  return usePaginatedQuery(\n")
			fmt.Fprintf(&sb, "    %s,\n", apiPath)
			if hasRequiredSkippable {
				sb.WriteString(g.generateArgsWithSpread(fn.Args, false))
			} else {
				// Wrap args in shouldSkip check via options
				argsLine := g.generateArgsWithSpread(fn.Args, false)
				trimmed := strings.TrimSuffix(strings.TrimPrefix(argsLine, "    "), ",\n")
				fmt.Fprintf(&sb, "    options?.shouldSkip ? \"skip\" : %s,\n", trimmed)
			}
			sb.WriteString("    { initialNumItems: options?.initialNumItems || 20 }\n")
			sb.WriteString("  );\n")
		} else {
			argsExpr, needsAsAnyCast := g.generateArgsWithSpreadInline(fn)
			// When typed returns are enabled, the function signature carries
			// `FunctionReturnType<typeof api.x.y> | undefined`, so the historical
			// `as any` return cast (used to swallow TS2589 deep-instantiation noise
			// during inference) is no longer needed and would erase the real type.
			if needsAsAnyCast && !g.config.DataLayer.TypedReturns {
				fmt.Fprintf(&sb, "  return useQuery(%s, %s) as any;\n", apiPath, argsExpr)
			} else {
				fmt.Fprintf(&sb, "  return useQuery(%s, %s);\n", apiPath, argsExpr)
			}
		}

	case FunctionTypeMutation:
		fmt.Fprintf(&sb, "  return useMutation(%s);\n", apiPath)

	case FunctionTypeAction:
		fmt.Fprintf(&sb, "  return useAction(%s);\n", apiPath)
	}

	return sb.String()
}

// generateArgsWithSpread creates args object with spread operator for optional params (multiline)
func (g *HooksGenerator) generateArgsWithSpread(args []ArgInfo, useShouldSkip bool) string {
	if len(args) == 0 {
		if useShouldSkip {
			return "    shouldSkip ? \"skip\" : {} as any,\n"
		}
		return "    {},\n"
	}

	// Find the primary skippable arg (first required ID)
	var primarySkipArg *ArgInfo
	for i := range args {
		if args[i].IsID && !args[i].Optional {
			primarySkipArg = &args[i]
			break
		}
	}

	// Build the args object with spread for optional params
	var argParts []string
	for _, arg := range args {
		if arg.Optional {
			// Use spread syntax for optional params
			argParts = append(argParts, fmt.Sprintf("...(%s !== null && %s !== undefined ? { %s } : {})", arg.Name, arg.Name, arg.Name))
		} else if arg.IsID && primarySkipArg != nil && arg.Name == primarySkipArg.Name {
			// Primary skip arg - include directly
			argParts = append(argParts, arg.Name)
		} else {
			argParts = append(argParts, arg.Name)
		}
	}

	if primarySkipArg != nil {
		return fmt.Sprintf("    %s ? { %s } as any : \"skip\",\n", primarySkipArg.Name, strings.Join(argParts, ", "))
	}

	if useShouldSkip {
		return fmt.Sprintf("    shouldSkip ? \"skip\" : { %s } as any,\n", strings.Join(argParts, ", "))
	}

	return fmt.Sprintf("    { %s },\n", strings.Join(argParts, ", "))
}

// generateArgsWithSpreadInline creates args object with spread operator for optional params (inline)
// Returns a tuple: (argsExpression, needsAsAnyCast)
func (g *HooksGenerator) generateArgsWithSpreadInline(fn ConvexFunction) (string, bool) {
	args := fn.Args
	hasRequiredSkippable := hasRequiredSkippableArg(args)

	if len(args) == 0 {
		if hasRequiredSkippable {
			return "{}", false
		}
		// shouldSkip pattern for no-args queries
		return "shouldSkip ? \"skip\" : {} as any", true
	}

	// Find the primary skippable args (required IDs)
	var primarySkipArgs []string
	for _, arg := range args {
		if arg.IsID && !arg.Optional {
			primarySkipArgs = append(primarySkipArgs, arg.Name)
		}
	}

	// Build the args object with spread for optional params
	var requiredArgs []string
	var optionalSpreads []string

	for _, arg := range args {
		if arg.Optional {
			// Use spread syntax for optional params
			optionalSpreads = append(optionalSpreads, fmt.Sprintf("...(%s !== null && %s !== undefined ? { %s } : {})", arg.Name, arg.Name, arg.Name))
		} else {
			requiredArgs = append(requiredArgs, arg.Name)
		}
	}

	// Combine required args and optional spreads
	var allParts []string
	allParts = append(allParts, requiredArgs...)
	allParts = append(allParts, optionalSpreads...)

	argsStr := strings.Join(allParts, ", ")

	if len(primarySkipArgs) > 0 {
		// Multiple skip conditions
		if len(primarySkipArgs) == 1 {
			return fmt.Sprintf("%s ? { %s } as any : \"skip\"", primarySkipArgs[0], argsStr), false
		}
		condition := strings.Join(primarySkipArgs, " && ")
		return fmt.Sprintf("%s ? { %s } as any : \"skip\"", condition, argsStr), false
	}

	// No required ID args - use shouldSkip pattern
	return fmt.Sprintf("shouldSkip ? \"skip\" : { %s } as any", argsStr), true
}

// toCamelCase converts a path like "voting/config" to "VotingConfig"
func toCamelCase(s string) string {
	parts := strings.Split(s, "/")
	for i, part := range parts {
		// Also handle underscores and convert camelCase
		subParts := strings.Split(part, "_")
		for j, sub := range subParts {
			subParts[j] = capitalize(sub)
		}
		parts[i] = strings.Join(subParts, "")
	}
	return strings.Join(parts, "")
}

// toNaturalLanguage converts camelCase to natural language
// e.g., "getEventCheckInList" -> "get event check in list"
func toNaturalLanguage(name string) string {
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune(' ')
			result.WriteRune(r + 32) // lowercase
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
