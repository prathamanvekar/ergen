package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func parseFuncDecl(src string) *ast.FuncDecl {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", "package main\n"+src, 0)
	if err != nil {
		panic(err)
	}
	return f.Decls[0].(*ast.FuncDecl)
}

func TestToHumanMessage(t *testing.T) {
	tests := []struct {
		funcName string
		expected string
	}{
		{"type assertion", "type assertion failed"},
		{"map lookup", "key not found in map"},
		{"channel receive", "channel closed"},
		{"assignment", "failed to assert value"},
		{"", "failed to assert value"},
		{"Open", "failed to open"},
		{"UnmarshalJSON", "failed to unmarshal json"},
		{"save_user_profile", "failed to save user profile"},
	}

	for _, tt := range tests {
		actual := ToHumanMessage(tt.funcName)
		if actual != tt.expected {
			t.Errorf("ToHumanMessage(%q) = %q; expected %q", tt.funcName, actual, tt.expected)
		}
	}
}

func TestGetZeroValue(t *testing.T) {
	// Setup custom package types
	typesMap := map[string]ast.Expr{
		"MyStatus": ast.NewIdent("int"),
		"MyStruct": &ast.StructType{},
	}

	tests := []struct {
		name     string
		exprSrc  string
		expected string
	}{
		{"int", "int", "0"},
		{"string", "string", `""`},
		{"bool", "bool", "false"},
		{"error", "error", "nil"},
		{"pointer", "*User", "nil"},
		{"slice", "[]string", "nil"},
		{"map", "map[string]int", "nil"},
		{"custom basic alias", "MyStatus", "MyStatus(0)"},
		{"custom struct", "MyStruct", "MyStruct{}"},
		{"imported selector struct", "models.User", "models.User{}"},
		{"time duration selector", "time.Duration", "0"},
	}

	for _, tt := range tests {
		expr, err := parser.ParseExpr(tt.exprSrc)
		if err != nil {
			t.Fatalf("failed to parse expr %s: %v", tt.exprSrc, err)
		}
		actual := getZeroValue(expr, typesMap)
		if actual != tt.expected {
			t.Errorf("getZeroValue for %s = %s; expected %s", tt.name, actual, tt.expected)
		}
	}
}

func TestAlignReturnStatement(t *testing.T) {
	tests := []struct {
		name         string
		funcDeclSrc  string
		template     string
		msg          string
		checkVarName string
		expectedPart string
	}{
		{
			name:         "Return only error",
			funcDeclSrc:  "func Foo() error",
			template:     "if err != nil {\n\treturn fmt.Errorf(\"{msg}: %w\", err)\n}",
			msg:          "failed to open",
			checkVarName: "err",
			expectedPart: `return fmt.Errorf("failed to open: %w", err)`,
		},
		{
			name:         "Return multiple values",
			funcDeclSrc:  "func Foo() (User, string, error)",
			template:     "if err != nil {\n\treturn fmt.Errorf(\"{msg}: %w\", err)\n}",
			msg:          "failed to fetch",
			checkVarName: "err",
			expectedPart: `return User{}, "", fmt.Errorf("failed to fetch: %w", err)`,
		},
		{
			name:         "Return nothing",
			funcDeclSrc:  "func Foo()",
			template:     "if err != nil {\n\treturn fmt.Errorf(\"{msg}: %w\", err)\n}",
			msg:          "failed to run",
			checkVarName: "err",
			expectedPart: "return",
		},
		{
			name:         "Check ok variable with return values",
			funcDeclSrc:  "func Foo() (string, error)",
			template:     "if err != nil {\n\treturn fmt.Errorf(\"{msg}: %w\", err)\n}",
			msg:          "key not found",
			checkVarName: "ok",
			expectedPart: `return "", errors.New("key not found")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enclosing := parseFuncDecl(tt.funcDeclSrc)
			res, err := AlignReturnStatement(tt.template, enclosing, tt.msg, tt.checkVarName, "dummy.go")
			if err != nil {
				t.Fatalf("AlignReturnStatement failed: %v", err)
			}
			if !strings.Contains(res, tt.expectedPart) {
				t.Errorf("expected aligned template to contain %q, but got:\n%s", tt.expectedPart, res)
			}
		})
	}
}
