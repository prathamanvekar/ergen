package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// TargetContext holds the collected metadata about the code block surrounding the target line.
type TargetContext struct {
	TargetLineNode    ast.Node      // The AST node representing the targeted assignment statement.
	EnclosingFunc     *ast.FuncDecl // The function decl containing the targeted statement.
	CalledFuncName    string        // Name of the function called on the RHS (e.g. "Open" from "os.Open").
	CallPackage       string        // Package prefix of the RHS call (e.g. "os" from "os.Open").
	CallFullPrefix    string        // Full selector/function name on the RHS (e.g. "os.Open").
	AssignmentEndLine int           // The 1-based line number where the assignment statement ends.
	CheckVarName      string        // The variable name to check (typically "err" or "ok").
}

// candidate represents a matching assignment statement found during AST inspection.
type candidate struct {
	assign       *ast.AssignStmt
	enclosing    *ast.FuncDecl
	matchType    int  // 1 if target line is within the assignment, 2 if target line is a blank line immediately after
	hasErr       bool // true if "err" is in the LHS of the assignment
	hasOk        bool // true if "ok" is in the LHS of the assignment
	distance     int  // absolute distance in lines from the target line to the assignment's end line
	endLine      int  // 1-based line where the assignment ends
}

// AnalyzeAST parses the Go source file, searches for candidate assignment nodes,
// selects the best candidate based on check-variable heuristics, and extracts RHS details.
func AnalyzeAST(filePath string, targetLine int) (*TargetContext, error) {
	fset := token.NewFileSet()

	// Parse file with comments preserved.
	fileNode, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// Read file contents to inspect if the target line is a blank line or contains comments.
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(fileBytes), "\n")
	isTargetLineBlank := false
	if targetLine > 0 && targetLine <= len(lines) {
		trimmed := strings.TrimSpace(lines[targetLine-1])
		isTargetLineBlank = (trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*"))
	}

	var currentFunc *ast.FuncDecl
	var candidates []candidate

	// Inspect AST looking for ast.AssignStmt nodes
	ast.Inspect(fileNode, func(n ast.Node) bool {
		if n == nil {
			return true
		}

		// Keep track of the current enclosing function declaration
		if funcDecl, ok := n.(*ast.FuncDecl); ok {
			currentFunc = funcDecl
		}

		// Identify assignments like `x, err := foo()`
		if assign, ok := n.(*ast.AssignStmt); ok {
			startLine := fset.Position(assign.Pos()).Line
			endLine := fset.Position(assign.End()).Line

			matchType := 0
			// Type 1: cursor is directly inside/on the assignment statement
			if targetLine >= startLine && targetLine <= endLine {
				matchType = 1
			} else if targetLine == endLine+1 && isTargetLineBlank {
				// Type 2: cursor is on the blank line immediately below the assignment statement
				matchType = 2
			}

			if matchType > 0 {
				hasErr := false
				hasOk := false
				// Check LHS variables to identify what check-variable to target (err vs ok)
				for _, expr := range assign.Lhs {
					if ident, ok := expr.(*ast.Ident); ok {
						if ident.Name == "err" {
							hasErr = true
						} else if ident.Name == "ok" {
							hasOk = true
						}
					}
				}

				distance := targetLine - endLine
				if distance < 0 {
					distance = -distance
				}

				candidates = append(candidates, candidate{
					assign:    assign,
					enclosing: currentFunc,
					matchType: matchType,
					hasErr:    hasErr,
					hasOk:     hasOk,
					distance:  distance,
					endLine:   endLine,
				})
			}
		}
		return true
	})

	if len(candidates) == 0 {
		return &TargetContext{}, nil
	}

	// Select the best candidate matching node using heuristics (Priority: MatchType 1 > hasErr > hasOk > minimal distance)
	best := candidates[0]
	for i := 1; i < len(candidates); i++ {
		if isBetterCandidate(candidates[i], best) {
			best = candidates[i]
		}
	}

	ctx := &TargetContext{
		TargetLineNode:    best.assign,
		EnclosingFunc:     best.enclosing,
		AssignmentEndLine: best.endLine,
	}

	if best.hasErr {
		ctx.CheckVarName = "err"
	} else if best.hasOk {
		ctx.CheckVarName = "ok"
	} else {
		ctx.CheckVarName = "err" // default fallback
	}

	// Extract the RHS call details
	if len(best.assign.Rhs) > 0 {
		ctx.CalledFuncName, ctx.CallPackage, ctx.CallFullPrefix = resolveRHSDetails(best.assign.Rhs[0])
	}

	return ctx, nil
}

// isBetterCandidate compares two candidate assignments and returns true if 'a' is a better target than 'b'.
func isBetterCandidate(a, b candidate) bool {
	if a.matchType != b.matchType {
		return a.matchType < b.matchType // 1 (within range) is better than 2 (on next blank line)
	}
	if a.hasErr != b.hasErr {
		return a.hasErr // Assignments checking 'err' take priority
	}
	if a.hasOk != b.hasOk {
		return a.hasOk // Assignments checking 'ok' take secondary priority
	}
	return a.distance < b.distance // Closest assignment wins
}

// resolveRHSDetails extracts function details, packages, and call prefixes from the RHS expression.
// Supports standard calls, selectors, type assertions, map index lookups, and channel receives.
func resolveRHSDetails(rhs ast.Expr) (funcName, pkg, fullPrefix string) {
	if rhs == nil {
		return "", "", ""
	}

	// Case 1: Standard call expressions (e.g. `foo()` or `pkg.Bar()`)
	if call, ok := rhs.(*ast.CallExpr); ok {
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			return fun.Name, "", fun.Name
		case *ast.SelectorExpr:
			pkgName := renderASTExpr(fun.X)
			return fun.Sel.Name, pkgName, renderASTExpr(fun)
		default:
			name := renderASTExpr(call.Fun)
			return name, "", name
		}
	}

	// Case 2: Non-function expressions requiring checks (type assert, map index, channel)
	switch rhs.(type) {
	case *ast.TypeAssertExpr:
		return "type assertion", "", ""
	case *ast.IndexExpr:
		return "map lookup", "", ""
	case *ast.UnaryExpr:
		if unary, ok := rhs.(*ast.UnaryExpr); ok && unary.Op == token.ARROW {
			return "channel receive", "", ""
		}
		return "assignment", "", ""
	default:
		return "assignment", "", ""
	}
}

// loadPackageTypes parses all non-test files in the same directory to build a map of package-declared types.
func loadPackageTypes(filePath string) (map[string]ast.Expr, error) {
	dir := filepath.Dir(filePath)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, err
	}

	typesMap := make(map[string]ast.Expr)
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				genDecl, ok := decl.(*ast.GenDecl)
				if !ok || genDecl.Tok != token.TYPE {
					continue
				}
				for _, spec := range genDecl.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if ok {
						typesMap[typeSpec.Name.Name] = typeSpec.Type
					}
				}
			}
		}
	}
	return typesMap, nil
}

// getZeroValue resolves any Go type (including custom types mapped in typesMap) to its string zero-value.
func getZeroValue(expr ast.Expr, typesMap map[string]ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr: // Pointer types like *User
		return "nil"
	case *ast.ArrayType: // Slices or arrays like []string
		return "nil"
	case *ast.MapType: // Maps like map[string]int
		return "nil"
	case *ast.ChanType: // Channels like chan bool
		return "nil"
	case *ast.FuncType: // Functions like func()
		return "nil"
	case *ast.InterfaceType: // Interfaces like interface{}
		return "nil"
	case *ast.Ident:
		name := t.Name
		switch name {
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "rune", "byte":
			return "0"
		case "float32", "float64":
			return "0.0"
		case "bool":
			return "false"
		case "string":
			return `""`
		case "error":
			return "nil"
		default:
			// Resolve custom local type declarations
			if underlying, ok := typesMap[name]; ok {
				underlyingZero := getZeroValue(underlying, typesMap)
				// If custom type is alias to primitive (e.g. MyStatus int), cast it cleanly: MyStatus(0)
				if underlyingZero == "0" || underlyingZero == "0.0" || underlyingZero == "false" || underlyingZero == `""` {
					return fmt.Sprintf("%s(%s)", name, underlyingZero)
				}
				if underlyingZero == "nil" {
					return "nil"
				}
				return name + "{}"
			}
			// Fallback: assume custom defined struct
			return name + "{}"
		}
	case *ast.SelectorExpr: // Qualified types (e.g. time.Duration, models.User)
		typeName := renderASTExpr(t)
		if typeName == "time.Duration" {
			return "0"
		}
		return typeName + "{}"
	case *ast.StructType: // Anonymous structs
		return "struct{}{}"
	default:
		return "nil"
	}
}

// AlignReturnStatement matches the template's return statement to the enclosing function's returns,
// inserting appropriate zero-values and re-parsing nodes to avoid formatting line wrapping/newlines.
func AlignReturnStatement(templateStr string, enclosingFunc *ast.FuncDecl, msg string, checkVarName string, filePath string) (string, error) {
	finalTemplate := strings.ReplaceAll(templateStr, "{msg}", msg)

	// Translate standard templates to boolean check variables when evaluating non-error assertions
	if checkVarName == "ok" {
		finalTemplate = strings.ReplaceAll(finalTemplate, "err != nil", "!ok")
		finalTemplate = strings.ReplaceAll(finalTemplate, "err", "ok")
		finalTemplate = strings.ReplaceAll(finalTemplate, "%w", "%v")
	}

	// If target line has no enclosing function, output template as-is
	if enclosingFunc == nil {
		return finalTemplate, nil
	}

	// Wrap in a dummy function so we can parse the template statement as valid Go source AST
	dummySrc := fmt.Sprintf("package main\nfunc _() {\n%s\n}", finalTemplate)
	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, "", dummySrc, 0)
	if err != nil {
		return finalTemplate, nil
	}

	var dummyFunc *ast.FuncDecl
	for _, decl := range fileNode.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == "_" {
			dummyFunc = fd
			break
		}
	}
	if dummyFunc == nil || dummyFunc.Body == nil {
		return finalTemplate, nil
	}

	// Load local package types
	typesMap, _ := loadPackageTypes(filePath)

	// Collect the enclosing function's return parameters
	var returnTypes []ast.Expr
	if enclosingFunc.Type.Results != nil {
		for _, field := range enclosingFunc.Type.Results.List {
			numNames := len(field.Names)
			if numNames == 0 {
				returnTypes = append(returnTypes, field.Type)
			} else {
				for i := 0; i < numNames; i++ {
					returnTypes = append(returnTypes, field.Type)
				}
			}
		}
	}

	// Locate position of the 'error' return type
	errIdx := -1
	for i, rt := range returnTypes {
		if isErrorType(rt) {
			errIdx = i
			break
		}
	}

	var walkErr error
	ast.Inspect(dummyFunc.Body, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		retStmt, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}

		// Enclosing function has no returns: rewrite return statements to empty returns
		if len(returnTypes) == 0 {
			retStmt.Results = nil
			return true
		}

		var errExpr ast.Expr
		if len(retStmt.Results) > 0 {
			// CRITICAL FIX: To prevent the "stray newline/line-wrap bug" when generating multi-line returns,
			// we render the error AST node to string and re-parse it with ParseExpr.
			// This strips the offset positioning data from the template parse session, ensuring
			// all return elements have uniform, clean starting positions so go/format aligns them on a single line.
			errExprStr := renderASTExpr(retStmt.Results[len(retStmt.Results)-1])
			errExprStr = strings.ReplaceAll(errExprStr, "\n", " ")
			errExprStr = strings.ReplaceAll(errExprStr, "\r", " ")
			parsedExpr, parseErr := parser.ParseExpr(errExprStr)
			if parseErr != nil {
				walkErr = fmt.Errorf("failed to re-parse error expression '%s': %w", errExprStr, parseErr)
				return false
			}
			errExpr = parsedExpr
		} else {
			if checkVarName == "ok" {
				errExpr = &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("errors"),
						Sel: ast.NewIdent("New"),
					},
					Args: []ast.Expr{
						&ast.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf(`"%s"`, msg),
						},
					},
				}
			} else {
				errExpr = ast.NewIdent("err")
			}
		}

		// Clean up error statements if returning after a type assert or map check (which uses 'ok')
		if checkVarName == "ok" && errIdx != -1 {
			if ident, ok := errExpr.(*ast.Ident); ok && ident.Name == "ok" {
				errExpr = &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   ast.NewIdent("errors"),
						Sel: ast.NewIdent("New"),
					},
					Args: []ast.Expr{
						&ast.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf(`"%s"`, msg),
						},
					},
				}
			} else if call, ok := errExpr.(*ast.CallExpr); ok {
				for i, arg := range call.Args {
					if ident, ok := arg.(*ast.Ident); ok && ident.Name == "ok" {
						call.Args[i] = &ast.BasicLit{
							Kind:  token.STRING,
							Value: `"failed to assert value"`,
						}
					}
				}
			}
		}

		// Map enclosing function results: insert zero-values and error values
		newResults := make([]ast.Expr, len(returnTypes))
		for i, rt := range returnTypes {
			if i == errIdx {
				newResults[i] = errExpr
			} else {
				zeroStr := getZeroValue(rt, typesMap)
				expr, parseErr := parser.ParseExpr(zeroStr)
				if parseErr != nil {
					walkErr = fmt.Errorf("failed to parse zero value '%s': %w", zeroStr, parseErr)
					return false
				}
				newResults[i] = expr
			}
		}
		retStmt.Results = newResults
		return true
	})

	if walkErr != nil {
		return "", walkErr
	}

		// Format back the rewritten AST block
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), dummyFunc.Body); err != nil {
		return "", fmt.Errorf("failed to format modified AST: %w", err)
	}

	bodyStr := buf.String()
	bodyStr = strings.TrimSpace(bodyStr)
	if strings.HasPrefix(bodyStr, "{") && strings.HasSuffix(bodyStr, "}") {
		bodyStr = bodyStr[1 : len(bodyStr)-1]
	}
	return strings.TrimSpace(bodyStr), nil
}

func isErrorType(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok && ident.Name == "error" {
		return true
	}
	return false
}

func renderASTExpr(expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return buf.String()
}

// ToHumanMessage converts a camelCase or snake_case function name into a lowercase, space-separated phrase.
// It also provides high-quality static fallbacks for type assertions, map index lookups, and channel receives.
func ToHumanMessage(funcName string) string {
	switch funcName {
	case "type assertion":
		return "type assertion failed"
	case "map lookup":
		return "key not found in map"
	case "channel receive":
		return "channel closed"
	case "assignment", "":
		return "failed to assert value"
	}

	var sb strings.Builder
	sb.WriteString("failed to ")

	runes := []rune(funcName)
	for i := 0; i < len(runes); i++ {
		current := runes[i]

		if current == '_' {
			if sb.Len() > 0 && !strings.HasSuffix(sb.String(), " ") {
				sb.WriteRune(' ')
			}
			continue
		}

		if i > 0 && unicode.IsUpper(current) {
			prev := runes[i-1]
			if unicode.IsLower(prev) || (i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				if !strings.HasSuffix(sb.String(), " ") {
					sb.WriteRune(' ')
				}
			}
		}

		sb.WriteRune(unicode.ToLower(current))
	}

	return sb.String()
}