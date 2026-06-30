package main

import (
	"testing"
)

func TestMatchRule(t *testing.T) {
	config := &Config{
		Rules: []Rule{
			{
				Name:              "test_handler",
				OuterHasParamType: "*testing.T",
				Template:          "t.Fatal(err)",
			},
			{
				Name:               "db_query",
				RhsCallPrefix:      "db.Query",
				OuterHasReturnType: "error",
				Template:           "return fmt.Errorf(\"db query: %w\", err)",
			},
			{
				Name:           "json_unmarshal",
				RhsCallPackage: "json",
				RhsCallName:    "Unmarshal",
				Template:       "return fmt.Errorf(\"failed json: %w\", err)",
			},
		},
	}

	// 1. Matches outer_has_param_type
	ctx1 := &TargetContext{
		EnclosingFunc: parseFuncDecl("func TestSomething(t *testing.T)"),
	}
	rule1 := config.MatchRule(ctx1)
	if rule1.Name != "test_handler" {
		t.Errorf("expected test_handler, got %s", rule1.Name)
	}

	// 2. Matches rhs_call_prefix and outer_has_return_type
	ctx2 := &TargetContext{
		EnclosingFunc:  parseFuncDecl("func Run() error"),
		CallFullPrefix: "db.QueryRow",
	}
	rule2 := config.MatchRule(ctx2)
	if rule2.Name != "db_query" {
		t.Errorf("expected db_query, got %s", rule2.Name)
	}

	// 3. Matches rhs_call_package and rhs_call_name
	ctx3 := &TargetContext{
		EnclosingFunc:  parseFuncDecl("func Parse() error"),
		CallPackage:    "json",
		CalledFuncName: "Unmarshal",
	}
	rule3 := config.MatchRule(ctx3)
	if rule3.Name != "json_unmarshal" {
		t.Errorf("expected json_unmarshal, got %s", rule3.Name)
	}

	// 4. Default fallback when no rule matches
	ctx4 := &TargetContext{
		EnclosingFunc: parseFuncDecl("func Generic() error"),
	}
	rule4 := config.MatchRule(ctx4)
	if rule4.Name != "default_fallback" {
		t.Errorf("expected default_fallback, got %s", rule4.Name)
	}
}
