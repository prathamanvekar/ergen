package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
)

// Rule represents a conditional block generation configuration mapping.
// Standard configuration keys inside .errgen.json map to this struct.
type Rule struct {
	Name               string `json:"name"`                  // A descriptive name of the matching rule.
	OuterHasParamType  string `json:"outer_has_param_type"`   // Pattern matching: parameters in the outer enclosing function signature.
	OuterHasReturnType string `json:"outer_has_return_type"`  // Pattern matching: return types in the outer enclosing function signature.
	RhsCallPrefix      string `json:"rhs_call_prefix"`        // Pattern matching: prefix of the RHS assignment call (e.g. "db.").
	RhsCallPackage     string `json:"rhs_call_package"`       // Pattern matching: direct package of the RHS call (e.g. "json").
	RhsCallName        string `json:"rhs_call_name"`          // Pattern matching: name of the RHS call function (e.g. "Unmarshal").
	Template           string `json:"template"`               // The error handling block template to inject. Supports '{msg}' formatting.
}

// Config wraps lists of match rules loaded from the settings file.
type Config struct {
	Rules []Rule `json:"rules"`
}

// MatchRule walks rules from top to bottom, adopting a "First Match Wins" precedence logic.
// All non-empty criteria within a single rule must match for the rule to trigger (AND logic).
func (c *Config) MatchRule(ctx *TargetContext) Rule {
	for _, rule := range c.Rules {
		matched := true

		// Check outer function parameter type criteria
		if rule.OuterHasParamType != "" {
			if !HasParamType(ctx.EnclosingFunc, rule.OuterHasParamType) {
				matched = false
			}
		}

		// Check outer function return type criteria
		if rule.OuterHasReturnType != "" {
			if !HasReturnType(ctx.EnclosingFunc, rule.OuterHasReturnType) {
				matched = false
			}
		}

		// Check RHS call selector/prefix criteria
		if rule.RhsCallPrefix != "" {
			if !strings.HasPrefix(ctx.CallFullPrefix, rule.RhsCallPrefix) {
				matched = false
			}
		}

		// Check RHS call package prefix criteria
		if rule.RhsCallPackage != "" {
			if ctx.CallPackage != rule.RhsCallPackage {
				matched = false
			}
		}

		// Check RHS call function name criteria
		if rule.RhsCallName != "" {
			if ctx.CalledFuncName != rule.RhsCallName {
				matched = false
			}
		}

		// If all specified criteria matched, use this rule template
		if matched {
			return rule
		}
	}

	// Default fallback rule used if no settings rule matches the target code's context
	return Rule{
		Name:     "default_fallback",
		Template: "if err != nil {\n\treturn fmt.Errorf(\"{msg}: %w\", err)\n}",
	}
}

// HasParamType checks if the enclosing function has a parameter matching the targetType.
func HasParamType(funcDecl *ast.FuncDecl, targetType string) bool {
	if funcDecl == nil || funcDecl.Type == nil || funcDecl.Type.Params == nil {
		return false
	}

	for _, field := range funcDecl.Type.Params.List {
		if matchType(field.Type, targetType) {
			return true
		}
	}
	return false
}

// HasReturnType checks if the enclosing function has a return type matching the targetType.
func HasReturnType(funcDecl *ast.FuncDecl, targetType string) bool {
	if funcDecl == nil || funcDecl.Type == nil || funcDecl.Type.Results == nil {
		return false
	}

	for _, field := range funcDecl.Type.Results.List {
		if matchType(field.Type, targetType) {
			return true
		}
	}
	return false
}

// matchType compares the rendered representation of an AST type expression with targetType.
// Standardizes comparison by handling star prefixes (e.g. matching *testing.T to testing.T).
func matchType(expr ast.Expr, targetType string) bool {
	rendered := renderASTExpr(expr)
	return rendered == targetType || strings.TrimPrefix(rendered, "*") == targetType
}

// LoadConfig reads and parses .errgen.json from the project root.
func LoadConfig(projectRoot string) (*Config, error) {
	configPath := filepath.Join(projectRoot, ".errgen.json")

	// If configuration does not exist, return an empty Config structure to safely use defaults
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return &Config{Rules: []Rule{}}, nil
	}

	bytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read .errgen.json: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(bytes, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse json config: %w", err)
	}

	return &cfg, nil
}

// FindProjectRoot climbs path trees to locate go.mod, indicating the root of the Go project.
func FindProjectRoot(targetFilePath string) (string, error) {
	absPath, err := filepath.Abs(targetFilePath)
	if err != nil {
		return "", err
	}

	currentDir := filepath.Dir(absPath)

	for {
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return currentDir, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			break
		}
		currentDir = parentDir
	}

	return "", fmt.Errorf("could not find project root containing go.mod")
}