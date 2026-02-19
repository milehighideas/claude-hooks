package main

import (
	"os"
	"regexp"
	"strings"
)

// FunctionType represents the type of Convex function
type FunctionType string

const (
	FunctionTypeQuery    FunctionType = "query"
	FunctionTypeMutation FunctionType = "mutation"
	FunctionTypeAction   FunctionType = "action"
)

// ConvexFunction represents a parsed Convex function
type ConvexFunction struct {
	Name            string
	Type            FunctionType
	Namespace       string       // Full namespace path (e.g., "events/voting")
	FileName        string       // Source file name
	Args            []ArgInfo    // Parsed arguments
	IsPaginated     bool         // Uses paginationOptsValidator
	UseFunctionArgs bool         // Args too complex, use FunctionArgs type
}

// ArgInfo represents a function argument
type ArgInfo struct {
	Name       string
	Type       string // TypeScript type
	Optional   bool
	IsID       bool   // Is v.id("table")
	IsArrayID  bool   // Is v.array(v.id("table"))
	TableName  string // For ID types, the table name
}

// TableInfo represents a parsed schema table
type TableInfo struct {
	Name       string // Table name in schema
	TypeName   string // PascalCase type name
	Domain     string // Schema domain/file
	FieldCount int    // Number of fields (approximate)
}

// Parser extracts information from TypeScript files
type Parser struct {
	config         *Config
	validatorCache map[string]string // Maps validator reference to its definition
}

// NewParser creates a new parser
func NewParser(config *Config) *Parser {
	return &Parser{
		config:         config,
		validatorCache: make(map[string]string),
	}
}

// BuildValidatorCache scans validator files and builds a cache of validator definitions
func (p *Parser) BuildValidatorCache(convexPath string) error {
	// Pattern to match: export const validatorName = v.object({ ... })
	validatorDefRe := regexp.MustCompile(`export\s+const\s+(\w+)\s*=\s*(v\.object\s*\([^;]+)`)


	// Walk through model directories looking for validators
	entries, err := os.ReadDir(convexPath)
	if err != nil {
		return err
	}


	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == "model" {
			modelPath := convexPath + "/model"
			p.scanValidatorDir(modelPath, validatorDefRe)
		}
	}

	return nil
}

// scanValidatorDir recursively scans a directory for validator definitions
func (p *Parser) scanValidatorDir(dirPath string, validatorDefRe *regexp.Regexp) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		fullPath := dirPath + "/" + entry.Name()
		if entry.IsDir() {
			p.scanValidatorDir(fullPath, validatorDefRe)
		} else if strings.HasSuffix(entry.Name(), "validators.ts") || strings.HasSuffix(entry.Name(), "validator.ts") {
			if strings.Contains(fullPath, "projects") {
			}
			p.parseValidatorFile(fullPath, validatorDefRe)
		}
	}
}

// parseValidatorFile extracts validator definitions from a file
func (p *Parser) parseValidatorFile(filePath string, validatorDefRe *regexp.Regexp) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	text := string(content)

	// Extract the module/namespace from the file path
	// e.g., "model/issues/validators.ts" -> "Issues"
	parts := strings.Split(filePath, "/")
	var namespace string
	for i, part := range parts {
		if part == "model" && i+1 < len(parts) {
			namespace = capitalize(parts[i+1])
			break
		}
	}

	if strings.Contains(filePath, "projects") {
		// Check if it contains getProjectValidator
		if strings.Contains(text, "getProjectValidator") {
		}
	}

	// Find all validator definitions
	matches := validatorDefRe.FindAllStringSubmatch(text, -1)

	if strings.Contains(filePath, "projects") {
	}

	for _, match := range matches {
		validatorName := match[1]
		validatorDef := match[2]

		if strings.Contains(validatorName, "getProjectValidator") {
		}

		// Store with full reference (e.g., "Issues.getIssueValidator")
		if namespace != "" {
			fullRef := namespace + "." + validatorName
			p.validatorCache[fullRef] = validatorDef
		}
		// Also store short name for local references
		p.validatorCache[validatorName] = validatorDef
	}
}

// Regex patterns for parsing
var (
	// Match: export const functionName = query({ ... }) or mutation({ ... }) or action({ ... })
	exportFunctionRe = regexp.MustCompile(`export\s+const\s+(\w+)\s*=\s*(query|mutation|action)\s*\(`)

	// Match internal functions (to skip)
	internalFunctionRe = regexp.MustCompile(`export\s+const\s+(\w+)\s*=\s+internal(Query|Mutation|Action)\s*\(`)

	// Match re-export pattern: export { func1, func2 } from './path'
	reExportRe = regexp.MustCompile(`export\s*\{([^}]+)\}\s*from\s*['"]([^'"]+)['"]`)

	// Match args: { ... } in the function config
	argsBlockRe = regexp.MustCompile(`args:\s*\{([^}]+)\}`)

	// Match args: SomeReference (e.g., args: Issues.getIssueValidator or args: getIssueValidator)
	argsRefRe = regexp.MustCompile(`args:\s*(\w+(?:\.\w+)?),?\s*(?:\n|handler)`)

	// Match individual arg: argName: v.something(...) OR argName: variableReference
	// The second pattern captures validator references that don't start with v.
	argRe = regexp.MustCompile(`(\w+):\s*(v\.[^,}]+|[a-zA-Z_]\w*)`)

	// Match v.id("tableName")
	idRe = regexp.MustCompile(`v\.id\(["'](\w+)["']\)`)

	// Match v.optional(v.id("tableName"))
	optionalIdRe = regexp.MustCompile(`v\.optional\(\s*v\.id\(["'](\w+)["']\)\s*\)`)

	// Match v.array(v.id("tableName"))
	arrayIdRe = regexp.MustCompile(`v\.array\(\s*v\.id\(["'](\w+)["']\)\s*\)`)

	// Match v.optional(v.array(v.id("tableName")))
	optionalArrayIdRe = regexp.MustCompile(`v\.optional\(\s*v\.array\(\s*v\.id\(["'](\w+)["']\)\s*\)\s*\)`)

	// Match simple types: v.string(), v.number(), v.boolean()
	primitiveRe = regexp.MustCompile(`^v\.(string|number|boolean|bigint)\(\)$`)

	// Match v.optional(v.primitive())
	optionalPrimitiveRe = regexp.MustCompile(`^v\.optional\(\s*v\.(string|number|boolean|bigint)\(\)\s*\)$`)

	// Match paginationOptsValidator - must have Validator suffix to be true Convex pagination
	paginationRe = regexp.MustCompile(`paginationOptsValidator`)

	// Match defineTable in schema
	defineTableRe = regexp.MustCompile(`(?:const\s+)?(\w+)\s*[:=]\s*defineTable\s*\(`)

	// Match table name in schema spread: ...tableTables
	spreadTableRe = regexp.MustCompile(`\.\.\.(\w+)Tables`)

	// Fluent-convex patterns

	// Match: export const NAME = IDENTIFIER (for fluent chain detection)
	fluentExportRe = regexp.MustCompile(`export\s+const\s+(\w+)\s*=\s*(\w+)`)

	// Match .input({ ... }) — for inline arg extraction
	inputBlockRe = regexp.MustCompile(`\.input\(\s*\{([^}]+)\}`)

	// Match .input(validatorRef) — for referenced validators
	inputRefRe = regexp.MustCompile(`\.input\(\s*(\w+(?:\.\w+)?)\s*\)`)
)

// Fluent-convex chain root → function type maps
var fluentQueryRoots = map[string]bool{
	"authedQuery": true, "userQuery": true, "adminQuery": true,
}
var fluentMutationRoots = map[string]bool{
	"authedMutation": true, "userMutation": true,
	"adminMutation": true, "superAdminMutation": true,
}
var fluentActionRoots = map[string]bool{
	"authedAction": true,
}

// ParseConvexFile extracts functions from a Convex TypeScript file
func (p *Parser) ParseConvexFile(file ConvexFile) ([]ConvexFunction, error) {
	if p.config.Convex.FluentConvex {
		return p.parseFluentConvexFile(file)
	}

	content, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}

	// Strip comments to avoid matching exports inside JSDoc examples
	text := stripComments(string(content))
	var functions []ConvexFunction

	if strings.Contains(file.Path, "projects.ts") && !strings.Contains(file.Path, "Internal") {
		if strings.Contains(string(content), "getProjectValidator") {
		}
		if strings.Contains(text, "getProjectValidator") {
		} else {
		}
	}

	// NOTE: Re-exports are intentionally NOT parsed.
	// Re-exports (e.g., `export { func } from './other'`) create duplicate function
	// entries which cause hook name collisions. The original functions in their
	// source files are parsed directly, so re-exports are unnecessary.

	// Find all exported functions
	matches := exportFunctionRe.FindAllStringSubmatchIndex(text, -1)

	for _, match := range matches {
		fullMatch := text[match[0]:match[1]]

		// Skip internal functions
		if internalFunctionRe.MatchString(fullMatch) {
			continue
		}

		funcName := text[match[2]:match[3]]
		funcType := FunctionType(text[match[4]:match[5]])

		// Extract the function body (find matching parenthesis)
		startIdx := match[1]
		funcBody := extractFunctionBody(text[startIdx:])

		if funcName == "getProject" && strings.Contains(file.Path, "projects.ts") {
			if len(funcBody) > 200 {
			} else {
			}
		}

		// Parse arguments
		args, isPaginated, useFunctionArgs := p.parseArgs(funcBody)

		functions = append(functions, ConvexFunction{
			Name:            funcName,
			Type:            funcType,
			Namespace:       file.Namespace,
			FileName:        file.FileName,
			Args:            args,
			IsPaginated:     isPaginated,
			UseFunctionArgs: useFunctionArgs,
		})
	}

	return functions, nil
}

// parseFluentConvexFile extracts functions from a fluent-convex style file.
// Fluent-convex uses builder chains like: export const name = adminQuery.input({...}).handler(...).public()
func (p *Parser) parseFluentConvexFile(file ConvexFile) ([]ConvexFunction, error) {
	content, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}

	text := stripComments(string(content))
	var functions []ConvexFunction

	// Find all "export const NAME = IDENTIFIER" patterns
	matches := fluentExportRe.FindAllStringSubmatchIndex(text, -1)

	for _, match := range matches {
		funcName := text[match[2]:match[3]]
		chainRoot := text[match[4]:match[5]]

		// Determine function type from the chain root identifier
		funcType := p.resolveFluentType(chainRoot, text[match[4]:])
		if funcType == "" {
			continue
		}

		// Extract the full chain text from the identifier to end of statement
		chainStart := match[4]
		chainText := p.extractFluentChain(text[chainStart:])
		if chainText == "" {
			continue
		}

		// Determine if public or internal
		isInternal := strings.Contains(chainText, ".internal()")
		isPublic := strings.Contains(chainText, ".public()")
		if !isInternal && !isPublic {
			// Not a registered function (could be a callable or middleware)
			continue
		}

		// Skip internal functions — only generate hooks for public ones
		if isInternal {
			continue
		}

		// Parse arguments from .input({...}) or .input(validatorRef)
		args, isPaginated, useFunctionArgs := p.parseFluentArgs(chainText)

		functions = append(functions, ConvexFunction{
			Name:            funcName,
			Type:            FunctionType(funcType),
			Namespace:       file.Namespace,
			FileName:        file.FileName,
			Args:            args,
			IsPaginated:     isPaginated,
			UseFunctionArgs: useFunctionArgs,
		})
	}

	return functions, nil
}

// resolveFluentType maps a fluent chain root identifier to a function type string.
// Returns "" if the identifier is not a recognized fluent chain root.
func (p *Parser) resolveFluentType(chainRoot, textFromExport string) string {
	// Direct chain root lookups
	if fluentQueryRoots[chainRoot] {
		return "query"
	}
	if fluentMutationRoots[chainRoot] {
		return "mutation"
	}
	if fluentActionRoots[chainRoot] {
		return "action"
	}

	// Base builder: "convex" — peek ahead for .query() / .mutation() / .action()
	if chainRoot == "convex" {
		if strings.Contains(textFromExport, ".query()") {
			return "query"
		}
		if strings.Contains(textFromExport, ".mutation()") {
			return "mutation"
		}
		if strings.Contains(textFromExport, ".action()") {
			return "action"
		}
	}

	return ""
}

// extractFluentChain extracts the full builder chain text from the chain root
// up to and including .public() or .internal(). Returns "" if neither is found.
func (p *Parser) extractFluentChain(text string) string {
	// Find .public() or .internal() to determine chain end
	publicIdx := strings.Index(text, ".public()")
	internalIdx := strings.Index(text, ".internal()")

	endIdx := -1
	if publicIdx >= 0 && internalIdx >= 0 {
		if publicIdx < internalIdx {
			endIdx = publicIdx + len(".public()")
		} else {
			endIdx = internalIdx + len(".internal()")
		}
	} else if publicIdx >= 0 {
		endIdx = publicIdx + len(".public()")
	} else if internalIdx >= 0 {
		endIdx = internalIdx + len(".internal()")
	}

	if endIdx == -1 {
		return ""
	}

	return text[:endIdx]
}

// parseFluentArgs extracts argument information from a fluent builder chain.
// Looks for .input({...}) inline blocks or .input(validatorRef) references.
func (p *Parser) parseFluentArgs(chainText string) ([]ArgInfo, bool, bool) {
	var args []ArgInfo
	isPaginated := false
	useFunctionArgs := false

	// Check for pagination
	if paginationRe.MatchString(chainText) {
		isPaginated = true
	}

	var argsBlock string

	// Try inline .input({ ... })
	if inputMatch := inputBlockRe.FindStringSubmatch(chainText); inputMatch != nil {
		argsBlock = inputMatch[1]
	} else if refMatch := inputRefRe.FindStringSubmatch(chainText); refMatch != nil {
		// Try .input(validatorRef)
		validatorRef := strings.TrimSpace(refMatch[1])
		var validatorDef string
		var found bool

		// First try exact match
		if validatorDef, found = p.validatorCache[validatorRef]; !found {
			// Try short name after last dot
			if dotIdx := strings.LastIndex(validatorRef, "."); dotIdx != -1 {
				shortName := validatorRef[dotIdx+1:]
				validatorDef, found = p.validatorCache[shortName]
			}
		}

		if found {
			innerMatch := regexp.MustCompile(`v\.object\s*\(\s*\{([^}]+)\}`).FindStringSubmatch(validatorDef)
			if innerMatch != nil {
				argsBlock = innerMatch[1]
			}
		}
	}

	if argsBlock == "" {
		return args, isPaginated, useFunctionArgs
	}

	// Reuse existing arg parsing logic
	return p.parseArgsBlock(argsBlock)
}

// parseArgsBlock parses a raw args block string (inner content of {...}) into ArgInfo.
// Shared between standard and fluent parsing paths.
func (p *Parser) parseArgsBlock(argsBlock string) ([]ArgInfo, bool, bool) {
	var args []ArgInfo
	isPaginated := false
	useFunctionArgs := false

	if paginationRe.MatchString(argsBlock) {
		isPaginated = true
	}

	argMatches := argRe.FindAllStringSubmatch(argsBlock, -1)
	for _, match := range argMatches {
		argName := match[1]
		argValidator := strings.TrimSpace(match[2])

		if argName == "paginationOpts" {
			continue
		}

		arg := p.parseArgValidator(argName, argValidator)

		if arg.Type == "unknown" {
			useFunctionArgs = true
		}

		args = append(args, arg)
	}

	if strings.Contains(argsBlock, "v.object(") || strings.Contains(argsBlock, "v.union(") {
		useFunctionArgs = true
	}

	return args, isPaginated, useFunctionArgs
}

// parseReExports handles re-export patterns like: export { func1, func2 } from './path'
// and resolves them to actual function definitions with the current file's namespace
func (p *Parser) parseReExports(file ConvexFile, text string) []ConvexFunction {
	var functions []ConvexFunction

	// Find all re-export statements
	matches := reExportRe.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		exportedNames := match[1]
		sourcePath := match[2]

		// Parse exported names (comma-separated, may have whitespace)
		names := strings.Split(exportedNames, ",")
		exportedSet := make(map[string]bool)
		for _, name := range names {
			name = strings.TrimSpace(name)
			// Handle aliased exports: originalName as aliasName
			if idx := strings.Index(name, " as "); idx != -1 {
				name = strings.TrimSpace(name[:idx])
			}
			if name != "" {
				exportedSet[name] = true
			}
		}

		// Resolve the source path relative to the current file
		sourceFilePath := p.resolveImportPath(file.Path, sourcePath)
		if sourceFilePath == "" {
			continue
		}

		// Read and parse the source file
		sourceContent, err := os.ReadFile(sourceFilePath)
		if err != nil {
			continue
		}

		sourceText := stripComments(string(sourceContent))

		// Find function definitions in source file
		funcMatches := exportFunctionRe.FindAllStringSubmatchIndex(sourceText, -1)
		for _, fm := range funcMatches {
			funcName := sourceText[fm[2]:fm[3]]
			funcType := FunctionType(sourceText[fm[4]:fm[5]])

			// Only include functions that are re-exported
			if !exportedSet[funcName] {
				continue
			}

			// Extract function body and parse args
			startIdx := fm[1]
			funcBody := extractFunctionBody(sourceText[startIdx:])
			args, isPaginated, useFunctionArgs := p.parseArgs(funcBody)

			// Use the re-exporting file's namespace, not the source file's
			functions = append(functions, ConvexFunction{
				Name:            funcName,
				Type:            funcType,
				Namespace:       file.Namespace,
				FileName:        file.FileName,
				Args:            args,
				IsPaginated:     isPaginated,
				UseFunctionArgs: useFunctionArgs,
			})
		}
	}

	return functions
}

// resolveImportPath resolves a relative import path to an absolute file path
func (p *Parser) resolveImportPath(currentFile, importPath string) string {
	// Get directory of current file
	dir := strings.TrimSuffix(currentFile, "/"+strings.Split(currentFile, "/")[len(strings.Split(currentFile, "/"))-1])

	// Handle relative imports
	if strings.HasPrefix(importPath, "./") {
		importPath = strings.TrimPrefix(importPath, "./")
	} else if strings.HasPrefix(importPath, "../") {
		// Handle parent directory references
		for strings.HasPrefix(importPath, "../") {
			importPath = strings.TrimPrefix(importPath, "../")
			dir = strings.TrimSuffix(dir, "/"+strings.Split(dir, "/")[len(strings.Split(dir, "/"))-1])
		}
	} else {
		// Not a relative import, skip
		return ""
	}

	// Construct full path
	fullPath := dir + "/" + importPath

	// Try with .ts extension
	if _, err := os.Stat(fullPath + ".ts"); err == nil {
		return fullPath + ".ts"
	}

	// Try as directory with index.ts
	if _, err := os.Stat(fullPath + "/index.ts"); err == nil {
		return fullPath + "/index.ts"
	}

	// Check if path already has extension
	if strings.HasSuffix(fullPath, ".ts") {
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}

	return ""
}

// extractFunctionBody finds the body between matching parentheses
func extractFunctionBody(text string) string {
	depth := 1 // Already past opening paren
	for i, ch := range text {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return text[:i]
			}
		}
	}
	return text
}

// parseArgs extracts argument information from function body
func (p *Parser) parseArgs(funcBody string) ([]ArgInfo, bool, bool) {
	var args []ArgInfo
	isPaginated := false
	useFunctionArgs := false

	if strings.Contains(funcBody, "getProjectValidator") {
		if len(funcBody) > 200 {
		} else {
		}
	}

	// Check for pagination
	if paginationRe.MatchString(funcBody) {
		isPaginated = true
	}

	// First try to find inline args block: args: { ... }
	argsMatch := argsBlockRe.FindStringSubmatch(funcBody)
	var argsBlock string

	if argsMatch != nil {
		argsBlock = argsMatch[1]
		if strings.Contains(funcBody, "getProjectValidator") {
		}
	} else {
		if strings.Contains(funcBody, "getProjectValidator") {
		}
		// Try to find referenced validator: args: SomeValidator or args: Module.validatorName
		refMatch := argsRefRe.FindStringSubmatch(funcBody)
		if strings.Contains(funcBody, "getProjectValidator") {
			if refMatch != nil {
			} else {
				// Try to manually find what's around "args:"
				idx := strings.Index(funcBody, "args:")
				if idx >= 0 {
					end := idx + 80
					if end > len(funcBody) {
						end = len(funcBody)
					}
				}
			}
		}
		if refMatch != nil {
			validatorRef := strings.TrimSpace(refMatch[1])
			var validatorDef string
			var found bool

			if strings.Contains(validatorRef, "getProject") {
			}

			// First try exact match
			if validatorDef, found = p.validatorCache[validatorRef]; !found {
				// If not found and has dot notation (e.g., ProjectModel.listProjectsValidator)
				// try looking up just the validator name after the last dot
				if dotIdx := strings.LastIndex(validatorRef, "."); dotIdx != -1 {
					shortName := validatorRef[dotIdx+1:]
					if strings.Contains(shortName, "getProject") {
					}
					validatorDef, found = p.validatorCache[shortName]
					if strings.Contains(shortName, "getProject") {
					}
				}
			}

			if found {
				if strings.Contains(validatorRef, "getProject") {
					// Print each character to see what's actually in there
					printLen := 100
					if len(validatorDef) < printLen {
						printLen = len(validatorDef)
					}
				}
				// Extract the inner content of v.object({ ... })
				innerMatch := regexp.MustCompile(`v\.object\s*\(\s*\{([^}]+)\}`).FindStringSubmatch(validatorDef)
				if innerMatch != nil {
					argsBlock = innerMatch[1]
					if strings.Contains(validatorRef, "getProject") {
					}
				} else if strings.Contains(validatorRef, "getProject") {
				}
			} else if strings.Contains(validatorRef, "getProject") {
			}
		}
	}

	if argsBlock == "" {
		return args, isPaginated, useFunctionArgs
	}

	// Parse each argument
	argMatches := argRe.FindAllStringSubmatch(argsBlock, -1)
	for _, match := range argMatches {
		argName := match[1]
		argValidator := strings.TrimSpace(match[2])

		// Skip pagination opts
		if argName == "paginationOpts" {
			continue
		}

		arg := p.parseArgValidator(argName, argValidator)

		// Check if we need FunctionArgs
		if arg.Type == "unknown" {
			useFunctionArgs = true
		}

		args = append(args, arg)
	}

	// If args block contains complex patterns, use FunctionArgs
	if strings.Contains(argsBlock, "v.object(") || strings.Contains(argsBlock, "v.union(") {
		useFunctionArgs = true
	}

	return args, isPaginated, useFunctionArgs
}

// parseArgValidator converts a validator string to ArgInfo
func (p *Parser) parseArgValidator(name, validator string) ArgInfo {
	arg := ArgInfo{
		Name: name,
		Type: "unknown",
	}

	// Check for optional patterns first
	if strings.HasPrefix(validator, "v.optional(") {
		arg.Optional = true
	}

	// Check v.optional(v.array(v.id("table")))
	if match := optionalArrayIdRe.FindStringSubmatch(validator); match != nil {
		arg.Type = "Id<\"" + match[1] + "\">[]"
		arg.IsID = true
		arg.IsArrayID = true
		arg.TableName = match[1]
		return arg
	}

	// Check v.array(v.id("table"))
	if match := arrayIdRe.FindStringSubmatch(validator); match != nil {
		arg.Type = "Id<\"" + match[1] + "\">[]"
		arg.IsID = true
		arg.IsArrayID = true
		arg.TableName = match[1]
		return arg
	}

	// Check v.optional(v.id("table"))
	if match := optionalIdRe.FindStringSubmatch(validator); match != nil {
		arg.Type = "Id<\"" + match[1] + "\">"
		arg.IsID = true
		arg.TableName = match[1]
		return arg
	}

	// Check v.id("table")
	if match := idRe.FindStringSubmatch(validator); match != nil {
		arg.Type = "Id<\"" + match[1] + "\">"
		arg.IsID = true
		arg.TableName = match[1]
		return arg
	}

	// Check v.optional(v.primitive())
	if match := optionalPrimitiveRe.FindStringSubmatch(validator); match != nil {
		arg.Type = match[1]
		return arg
	}

	// Check v.primitive()
	if match := primitiveRe.FindStringSubmatch(validator); match != nil {
		arg.Type = match[1]
		return arg
	}

	// Check for array of primitives
	if strings.Contains(validator, "v.array(v.string())") {
		arg.Type = "string[]"
		return arg
	}
	if strings.Contains(validator, "v.array(v.number())") {
		arg.Type = "number[]"
		return arg
	}

	return arg
}

// ParseSchemaFile extracts table definitions from a schema file
func (p *Parser) ParseSchemaFile(file SchemaFile) ([]TableInfo, error) {
	content, err := os.ReadFile(file.Path)
	if err != nil {
		return nil, err
	}

	text := string(content)
	var tables []TableInfo

	// Check if this is the main schema file (contains defineSchema)
	if strings.Contains(text, "defineSchema(") {
		// Parse the main schema file - extract table names from defineSchema keys
		tables = p.parseMainSchemaFile(text)
	} else {
		// Individual schema file - use defineTable variable names
		matches := defineTableRe.FindAllStringSubmatch(text, -1)
		for _, match := range matches {
			tableName := match[1]

			tables = append(tables, TableInfo{
				Name:     tableName,
				TypeName: toPascalCase(tableName),
				Domain:   file.Domain,
			})
		}
	}

	return tables, nil
}

// parseMainSchemaFile extracts table names from defineSchema() in the main schema file
func (p *Parser) parseMainSchemaFile(text string) []TableInfo {
	var tables []TableInfo

	// Find the defineSchema block - need to handle nested braces
	startIdx := strings.Index(text, "defineSchema({")
	if startIdx == -1 {
		// Try alternate format with whitespace
		defineSchemaRe := regexp.MustCompile(`defineSchema\s*\(\s*\{`)
		loc := defineSchemaRe.FindStringIndex(text)
		if loc == nil {
			return tables
		}
		startIdx = loc[0]
	}

	// Find the opening brace of defineSchema({
	braceIdx := strings.Index(text[startIdx:], "{")
	if braceIdx == -1 {
		return tables
	}
	braceIdx += startIdx

	// Extract the content between braces
	depth := 1
	endIdx := braceIdx + 1
	for endIdx < len(text) && depth > 0 {
		switch text[endIdx] {
		case '{':
			depth++
		case '}':
			depth--
		}
		endIdx++
	}

	if depth != 0 {
		return tables
	}

	schemaBlock := text[braceIdx+1 : endIdx-1]

	// Count spread operators vs direct table entries
	// If there are many spreads, fall back to individual file parsing
	lines := strings.Split(schemaBlock, "\n")
	spreadCount := 0
	directCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "...") {
			spreadCount++
		} else if isValidIdentifier(strings.Split(strings.TrimSuffix(line, ","), ":")[0]) {
			directCount++
		}
	}

	// If most entries are spreads, fall back to individual file parsing
	if spreadCount > 5 && spreadCount > directCount {
		return tables
	}

	// Parse each line for table entries
	// Formats:
	// - "tableName," - simple table name (used as both key and value)
	// - "tableName: variableName," - explicit key: value mapping
	// - "...spread" - skip spread operators (external tables)
	// - "// comment" - skip comments

	seen := make(map[string]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines, comments, and spread operators
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "...") {
			continue
		}

		// Check for "tableName: variableName" or "tableName," pattern
		// Match the table name (key before colon, or identifier before comma)
		var tableName string

		if colonIdx := strings.Index(line, ":"); colonIdx != -1 {
			// Has colon - table name is before the colon
			tableName = strings.TrimSpace(line[:colonIdx])
		} else if commaIdx := strings.Index(line, ","); commaIdx != -1 {
			// Just a comma - table name is before the comma
			tableName = strings.TrimSpace(line[:commaIdx])
		} else {
			// Last entry without trailing comma
			tableName = strings.TrimSpace(line)
		}

		// Validate table name is a valid identifier
		if tableName == "" || !isValidIdentifier(tableName) {
			continue
		}

		// Skip duplicates
		if seen[tableName] {
			continue
		}
		seen[tableName] = true

		tables = append(tables, TableInfo{
			Name:     tableName,
			TypeName: toPascalCase(tableName),
			Domain:   "main",
		})
	}

	return tables
}

// isValidIdentifier checks if a string is a valid JS/TS identifier
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, ch := range s {
		if i == 0 {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '$') {
				return false
			}
		} else {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '$') {
				return false
			}
		}
	}
	return true
}

// stripComments removes block comments (/* ... */) and line comments (// ...)
// to prevent matching exports inside JSDoc examples
func stripComments(text string) string {
	var result strings.Builder
	i := 0
	for i < len(text) {
		// Check for block comment start
		if i+1 < len(text) && text[i] == '/' && text[i+1] == '*' {
			// Skip until end of block comment
			i += 2
			for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
				// Preserve newlines to maintain line structure
				if text[i] == '\n' {
					result.WriteByte('\n')
				}
				i++
			}
			i += 2 // Skip */
			continue
		}
		// Check for line comment
		if i+1 < len(text) && text[i] == '/' && text[i+1] == '/' {
			// Skip until end of line
			for i < len(text) && text[i] != '\n' {
				i++
			}
			continue
		}
		// Regular character
		if i < len(text) {
			result.WriteByte(text[i])
		}
		i++
	}
	return result.String()
}

// toPascalCase converts a string to PascalCase
func toPascalCase(s string) string {
	if s == "" {
		return s
	}

	// Handle snake_case
	if strings.Contains(s, "_") {
		parts := strings.Split(s, "_")
		for i, part := range parts {
			if len(part) > 0 {
				parts[i] = strings.ToUpper(part[:1]) + part[1:]
			}
		}
		return strings.Join(parts, "")
	}

	// Simple capitalize first letter
	return strings.ToUpper(s[:1]) + s[1:]
}
