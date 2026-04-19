package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// --- Unit tests for sanitizeCLIName ---

// TestSanitizeCLIName_DotRejected verifies that "." is rejected as invalid.
func TestSanitizeCLIName_DotRejected(t *testing.T) {
	_, err := sanitizeCLIName(".")
	if err == nil {
		t.Fatal("sanitizeCLIName('.') must return an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CLI name") {
		t.Errorf("error must contain 'invalid CLI name', got: %q", err.Error())
	}
}

// TestSanitizeCLIName_DotDotRejected verifies that ".." is rejected as invalid.
func TestSanitizeCLIName_DotDotRejected(t *testing.T) {
	_, err := sanitizeCLIName("..")
	if err == nil {
		t.Fatal("sanitizeCLIName('..') must return an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CLI name") {
		t.Errorf("error must contain 'invalid CLI name', got: %q", err.Error())
	}
}

// TestSanitizeCLIName_AllSpecialCharsRejected verifies that "!!!" becomes empty
// after sanitizing and is therefore rejected.
func TestSanitizeCLIName_AllSpecialCharsRejected(t *testing.T) {
	_, err := sanitizeCLIName("!!!")
	if err == nil {
		t.Fatal("sanitizeCLIName('!!!') must return an error (empty after sanitize), got nil")
	}
	if !strings.Contains(err.Error(), "invalid CLI name") {
		t.Errorf("error must contain 'invalid CLI name', got: %q", err.Error())
	}
}

// TestSanitizeCLIName_DotDotSlashNeutralized verifies that "../evil" is neutralized
// to "evil" (no error) by stripping the leading "../" via ToCommandName.
func TestSanitizeCLIName_DotDotSlashNeutralized(t *testing.T) {
	result, err := sanitizeCLIName("../evil")
	if err != nil {
		t.Fatalf("sanitizeCLIName('../evil') must succeed (neutralized), got error: %v", err)
	}
	if result != "evil" {
		t.Errorf("sanitizeCLIName('../evil') must return 'evil', got %q", result)
	}
}

// TestSanitizeCLIName_SpacesNormalized verifies that "My API" becomes "my-api".
func TestSanitizeCLIName_SpacesNormalized(t *testing.T) {
	result, err := sanitizeCLIName("My API")
	if err != nil {
		t.Fatalf("sanitizeCLIName('My API') must succeed, got error: %v", err)
	}
	if result != "my-api" {
		t.Errorf("sanitizeCLIName('My API') must return 'my-api', got %q", result)
	}
}

// TestSanitizeCLIName_ErrorFormat verifies the exact error format "invalid CLI name %q".
func TestSanitizeCLIName_ErrorFormat(t *testing.T) {
	_, err := sanitizeCLIName(".")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error must contain: invalid CLI name "."
	want := `invalid CLI name "."`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error format mismatch; want substring %q, got %q", want, err.Error())
	}
}

// --- Integration test: addCmd rejects dot/dotdot --name values ---

// TestAddCmd_DotNameRejected verifies that addCmd RunE returns an error containing
// "invalid CLI name" when --name "." is supplied.
func TestAddCmd_DotNameRejected(t *testing.T) {
	isolatedHome(t)
	specPath := writeSpec(t, minimalSwaggerSpec)

	cmd := addCmd()
	cmd.SetArgs([]string{"--name", ".", specPath})

	// Suppress output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()

	if err == nil {
		t.Fatal("addCmd with --name '.' must return an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CLI name") {
		t.Errorf("error must contain 'invalid CLI name', got: %q", err.Error())
	}
}

// TestAddCmd_DotDotNameRejected verifies that addCmd RunE returns an error containing
// "invalid CLI name" when --name ".." is supplied.
func TestAddCmd_DotDotNameRejected(t *testing.T) {
	isolatedHome(t)
	specPath := writeSpec(t, minimalSwaggerSpec)

	cmd := addCmd()
	cmd.SetArgs([]string{"--name", "..", specPath})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()

	if err == nil {
		t.Fatal("addCmd with --name '..' must return an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CLI name") {
		t.Errorf("error must contain 'invalid CLI name', got: %q", err.Error())
	}
}

// TestAddCmd_DotNameRejectedBeforeDryRun verifies that addCmd rejects dot names
// even when --dry-run is also passed (sanitize happens before dryRun branch).
func TestAddCmd_DotNameRejectedBeforeDryRun(t *testing.T) {
	isolatedHome(t)
	specPath := writeSpec(t, minimalSwaggerSpec)

	cmd := addCmd()
	cmd.SetArgs([]string{"--dry-run", "--name", ".", specPath})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	buf.ReadFrom(r)
	r.Close()

	if err == nil {
		t.Fatal("addCmd --dry-run --name '.' must return an error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid CLI name") {
		t.Errorf("error must contain 'invalid CLI name', got: %q", err.Error())
	}
}

// --- Source-scan assertion: no raw data.Name = name assignment remains ---

// TestNoRawDataNameAssignment verifies that main.go does not contain any raw
// 'data.Name = name' assignment for the --name flag path (the vulnerability pattern).
func TestNoRawDataNameAssignment(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	content := string(src)
	// The vulnerable pattern is a bare assignment without sanitization.
	if strings.Contains(content, "data.Name = name") {
		t.Error("main.go must not contain 'data.Name = name' — use sanitizeCLIName first")
	}
}

// TestSanitizeCLINameFuncPresent verifies that sanitizeCLIName is defined in main.go.
func TestSanitizeCLINameFuncPresent(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	if !strings.Contains(string(src), "func sanitizeCLIName(") {
		t.Error("main.go must define sanitizeCLIName function")
	}
}

// TestToCommandNameExported verifies that ToCommandName (exported) is used in main.go
// via the sanitizeCLIName helper, confirming the rename happened.
func TestToCommandNameExported(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	if !strings.Contains(string(src), "parser.ToCommandName") {
		t.Error("main.go must call parser.ToCommandName (exported) inside sanitizeCLIName")
	}
}
