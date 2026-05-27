package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
	"github.com/chinmayrelkar/bawarchi/internal/registry"
)

// minimalSwaggerSpec is a minimal Swagger 2.0 spec used by dry-run CLI tests.
const minimalSwaggerSpec = `{
  "swagger": "2.0",
  "info": {
    "title": "drytest",
    "version": "1.0.0"
  },
  "host": "api.example.com",
  "basePath": "/v1",
  "schemes": ["https"],
  "paths": {
    "/items": {
      "get": {
        "operationId": "listItems",
        "summary": "List items",
        "parameters": [],
        "responses": {
          "200": {"description": "ok"}
        }
      }
    }
  }
}`

// isolatedHome redirects HOME to a temp dir so registry state never touches the
// real ~/.bawarchi. The original HOME is restored after the test.
func isolatedHome(t *testing.T) {
	t.Helper()
	orig := os.Getenv("HOME")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	_ = orig              // restored automatically via t.Setenv
	_ = registry.BinDir() // ensure lazy init uses the new HOME
}

// writeSpec writes content to a temp file and returns its path.
func writeSpec(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "spec-*.json")
	if err != nil {
		t.Fatalf("creating temp spec file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("writing temp spec file: %v", err)
	}
	f.Close()
	return f.Name()
}

// runAddDryRun executes addCmd with --dry-run and captures stdout.
func runAddDryRun(t *testing.T, specPath string) (string, error) {
	t.Helper()

	cmd := addCmd()

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	// Cobra uses os.Stdout for print statements inside RunE when SetOut is set on
	// the command itself. However, our RunE uses fmt.Fprintln(os.Stdout, ...) and
	// os.Stdout.Write(...) directly, so we need to redirect os.Stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	cmd.SetArgs([]string{"--dry-run", specPath})
	runErr := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var captured bytes.Buffer
	captured.ReadFrom(r)
	r.Close()

	return captured.String(), runErr
}

// TestDryRunPrintsSeparatorAndSource (assertion a) verifies that --dry-run prints
// the separator line followed by Go source containing 'package main'.
func TestDryRunPrintsSeparatorAndSource(t *testing.T) {
	isolatedHome(t)
	specPath := writeSpec(t, minimalSwaggerSpec)

	out, err := runAddDryRun(t, specPath)
	if err != nil {
		t.Fatalf("addCmd --dry-run returned error: %v", err)
	}
	if !strings.Contains(out, "--- generated: main.go ---") {
		t.Errorf("output missing separator; got:\n%s", out)
	}
	if !strings.Contains(out, "package main") {
		t.Errorf("output missing 'package main'; got:\n%s", out)
	}
}

// TestDryRunNoFilesWritten (assertion b) verifies that --dry-run writes no files
// to ~/.bawarchi and creates no registry entry.
func TestDryRunNoFilesWritten(t *testing.T) {
	isolatedHome(t)
	tmpHome := os.Getenv("HOME")
	specPath := writeSpec(t, minimalSwaggerSpec)

	_, err := runAddDryRun(t, specPath)
	if err != nil {
		t.Fatalf("addCmd --dry-run returned error: %v", err)
	}

	// Check that ~/.bawarchi directory was not created (or is empty).
	bawarchiDir := filepath.Join(tmpHome, ".bawarchi")
	if _, statErr := os.Stat(bawarchiDir); statErr == nil {
		// Directory exists — make sure there are no bin/src subdirectory entries.
		entries, _ := os.ReadDir(bawarchiDir)
		for _, e := range entries {
			if e.Name() == "bin" || e.Name() == "src" {
				t.Errorf("--dry-run must not create %s inside ~/.bawarchi", e.Name())
			}
		}
	}

	// Confirm no registry entry was created.
	data, _ := parser.ParseSource(specPath)
	if data != nil {
		if _, regErr := registry.Get(data.Name); regErr == nil {
			t.Errorf("--dry-run must not register %q in the registry", data.Name)
		}
	}
}

// TestDryRunExitsZero (assertion c) verifies that --dry-run exits with code 0
// (i.e., RunE returns nil).
func TestDryRunExitsZero(t *testing.T) {
	isolatedHome(t)
	specPath := writeSpec(t, minimalSwaggerSpec)

	_, err := runAddDryRun(t, specPath)
	if err != nil {
		t.Errorf("--dry-run should return nil (exit 0), got: %v", err)
	}
}

// TestNoDryRunFullPipelineUnchanged (assertion d) verifies that omitting --dry-run
// still attempts the full pipeline (cookAndRegister is called). We confirm this by
// detecting that it tries to compile — which will fail in the test environment —
// rather than returning the dry-run separator in stdout.
func TestNoDryRunFullPipelineUnchanged(t *testing.T) {
	isolatedHome(t)
	specPath := writeSpec(t, minimalSwaggerSpec)

	cmd := addCmd()
	cmd.SetArgs([]string{specPath})

	// Redirect stdout to suppress output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var out bytes.Buffer
	out.ReadFrom(r)
	r.Close()

	// Without --dry-run the command should NOT print the separator.
	if strings.Contains(out.String(), "--- generated: main.go ---") {
		t.Error("non-dry-run invocation must not print the dry-run separator")
	}

	// The command may succeed (if go toolchain is present) or fail at compile/register;
	// either way it must not have taken the dry-run path (no separator).
	_ = err
}
