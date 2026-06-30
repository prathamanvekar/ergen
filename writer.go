package main

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/token"
	"os"

	"golang.org/x/tools/imports"
)

// InjectErrorBlock inserts the templateStr code block into filePath immediately after insertAfterLine.
// It formats the code, resolves missing imports, and performs an atomic syntax verification check before writing to disk.
func InjectErrorBlock(filePath string, insertAfterLine int, templateStr string) error {
	// Read original source file content.
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading target file: %w", err)
	}

	// Split code into lines to insert the block at the correct line offset.
	lines := bytes.Split(fileBytes, []byte("\n"))
	updatedLines := make([][]byte, 0, len(lines)+2)

	// Build mutated buffer, injecting error template after the end of the assignment statement.
	for i, line := range lines {
		updatedLines = append(updatedLines, line)

		// insertAfterLine is 1-based index, corresponding to index insertAfterLine-1 in 0-based slice.
		if i == insertAfterLine-1 {
			updatedLines = append(updatedLines, []byte(templateStr))
		}
	}

	outputBytes := bytes.Join(updatedLines, []byte("\n"))

	// Format code and resolve imports dynamically (equivalent to running goimports).
	formattedBytes, err := imports.Process(filePath, outputBytes, nil)
	if err != nil {
		return fmt.Errorf("failed to format and resolve imports: %w", err)
	}

	// SECURITY/ROBUSTNESS GUARD: Perform a pre-write syntax validation scan.
	// Parse the formatted code in-memory to ensure it compiles structurally.
	// This guarantees that a corrupted template or injection bug will never damage/wipe a user's original source file.
	fset := token.NewFileSet()
	if _, parseErr := parser.ParseFile(fset, filePath, formattedBytes, 0); parseErr != nil {
		return fmt.Errorf("safety block aborted write: mutated output contains syntax errors: %w", parseErr)
	}

	// Write mutated and validated Go source code safely back to disk.
	if err := os.WriteFile(filePath, formattedBytes, 0644); err != nil {
		return fmt.Errorf("failed to write mutated file to disk: %w", err)
	}

	return nil
}