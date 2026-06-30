package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Parse input command line flags specifying target file and target cursor line.
	filePath := flag.String("file", "", "the file path of the file to analyze")
	lineNumber := flag.Int("line", 0, "the line number where the cursor is placed")

	flag.Parse()

	// Perform basic input validation.
	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "Error: file path is required")
		os.Exit(0) // Exit with code 0 to prevent editor popups
	}
	if *lineNumber <= 0 {
		fmt.Fprintln(os.Stderr, "Error: line number must be greater than 0")
		os.Exit(0) // Exit with code 0 to prevent editor popups
	}

	// SECURITY GUARD: Sanitize and clean incoming target filepath to prevent directory traversal.
	cleanPath := filepath.Clean(*filePath)

	// SECURITY GUARD: Ensure we only attempt static analysis on valid Go source code files.
	if !strings.HasSuffix(cleanPath, ".go") {
		fmt.Fprintf(os.Stderr, "Error: target file '%s' is not a Go (.go) source file\n", cleanPath)
		os.Exit(0)
	}

	// SECURITY GUARD: Verify target file exists, is readable, and is not a directory.
	info, err := os.Stat(cleanPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to access target file: %v\n", err)
		os.Exit(0)
	}
	if info.IsDir() {
		fmt.Fprintln(os.Stderr, "Error: target path cannot be a directory")
		os.Exit(0)
	}

	// Step 1: Locate the project's root module directory (contains go.mod).
	// We need the root to lookup target .errgen.json configuration rules.
	projectRootPath, err := FindProjectRoot(cleanPath)
	if err != nil {
		// Log to stderr and exit gracefully to prevent editor notifications/panics.
		fmt.Fprintf(os.Stderr, "Warning: failed to find project root: %v\n", err)
		os.Exit(0)
	}

	// Step 2: Load pattern matching configuration rules from .errgen.json.
	config, err := LoadConfig(projectRootPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load config (using fallback defaults): %v\n", err)
		// Load empty configuration to fallback cleanly
		config = &Config{Rules: []Rule{}}
	}

	// Step 3: Run the AST inspector over the target file.
	// We seek the closest target assignment candidate statement matching the line offset.
	ctx, err := AnalyzeAST(cleanPath, *lineNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to analyze AST: %v\n", err)
		os.Exit(0)
	}

	// Graceful Exit: Terminate silently if no assignment statement was found matching target line heuristics.
	if ctx.AssignmentEndLine == 0 {
		fmt.Fprintf(os.Stderr, "No target assignment found on or near line %d\n", *lineNumber)
		os.Exit(0)
	}

	// Step 4: Resolve the matching rule from config (First Match Wins).
	matchedRule := config.MatchRule(ctx)
	humanMsg := ToHumanMessage(ctx.CalledFuncName)

	// Step 5: Align the template's return statement to the outer enclosing function signature.
	// This generates zero-values dynamically and normalizes node offsets to prevent newline formatting anomalies.
	alignedTemplate, err := AlignReturnStatement(matchedRule.Template, ctx.EnclosingFunc, humanMsg, ctx.CheckVarName, cleanPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to align return statement: %v\n", err)
		os.Exit(0)
	}

	// Step 6: Inject the formatted code block into the file right below the assignment statement.
	err = InjectErrorBlock(cleanPath, ctx.AssignmentEndLine, alignedTemplate)
	if err != nil {
		log.Fatalf("Error: failed to mutate source file safely: %v", err)
	}

	fmt.Printf("Generated '%s' block successfully under line %d (assignment end line %d).\n", matchedRule.Name, *lineNumber, ctx.AssignmentEndLine)
}