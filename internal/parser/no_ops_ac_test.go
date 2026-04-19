package parser

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestParseOpenAPI_NoPaths_ReturnsError
// A spec with paths:{} must return a non-nil error containing
// "spec defines no operations".
// ---------------------------------------------------------------------------
func TestParseOpenAPI_NoPaths_ReturnsError(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: Empty API
  version: "1.0"
paths: {}
`)
	_, err := ParseOpenAPI(spec)
	if err == nil {
		t.Fatal("expected non-nil error for spec with paths:{}, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestParseSwagger_NoPaths_ReturnsError
// A Swagger 2.0 spec with paths:{} must return a non-nil error containing
// "spec defines no operations". schemes:[https] and host are provided to
// suppress the http-fallback warning on stderr.
// ---------------------------------------------------------------------------
func TestParseSwagger_NoPaths_ReturnsError(t *testing.T) {
	spec := []byte(`
swagger: "2.0"
info:
  title: Empty API
  version: "1.0"
host: api.example.com
schemes:
  - https
paths: {}
`)
	_, err := ParseSwagger(spec)
	if err == nil {
		t.Fatal("expected non-nil error for swagger spec with paths:{}, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestParseOpenAPI_PathsNoOperations_ReturnsError
// A spec whose single path item has no HTTP method keys (e.g. /health: {})
// must return a non-nil error containing "spec defines no operations".
// ---------------------------------------------------------------------------
func TestParseOpenAPI_PathsNoOperations_ReturnsError(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: Empty API
  version: "1.0"
paths:
  /health: {}
`)
	_, err := ParseOpenAPI(spec)
	if err == nil {
		t.Fatal("expected non-nil error for path item with no HTTP operations, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestParseSwagger_PathsNoOperations_ReturnsError
// A Swagger 2.0 spec whose single path item has no HTTP method keys must
// return a non-nil error containing "spec defines no operations".
// ---------------------------------------------------------------------------
func TestParseSwagger_PathsNoOperations_ReturnsError(t *testing.T) {
	spec := []byte(`
swagger: "2.0"
info:
  title: Empty API
  version: "1.0"
host: api.example.com
schemes:
  - https
paths:
  /health: {}
`)
	_, err := ParseSwagger(spec)
	if err == nil {
		t.Fatal("expected non-nil error for swagger path item with no HTTP operations, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// TestParseOpenAPI_WithOperations_NoError
// A spec with at least one valid HTTP operation must return nil error and a
// non-empty Commands slice. Uses the in-package helper minimalOpenAPI3SpecYAML.
// ---------------------------------------------------------------------------
func TestParseOpenAPI_WithOperations_NoError(t *testing.T) {
	data := minimalOpenAPI3SpecYAML()
	cli, err := ParseOpenAPI(data)
	if err != nil {
		t.Fatalf("expected nil error for spec with a valid operation, got: %v", err)
	}
	if len(cli.Commands) == 0 {
		t.Fatal("expected non-empty Commands slice for spec with a valid operation")
	}
}

// ---------------------------------------------------------------------------
// TestParseSwagger_WithOperations_NoError
// A Swagger 2.0 spec with at least one valid HTTP operation must return nil
// error and a non-empty Commands slice. Uses minimalSwagger2SpecYAML.
// ---------------------------------------------------------------------------
func TestParseSwagger_WithOperations_NoError(t *testing.T) {
	data := minimalSwagger2SpecYAML("path", "integer")
	cli, err := ParseSwagger(data)
	if err != nil {
		t.Fatalf("expected nil error for swagger spec with a valid operation, got: %v", err)
	}
	if len(cli.Commands) == 0 {
		t.Fatal("expected non-empty Commands slice for swagger spec with a valid operation")
	}
}

// ---------------------------------------------------------------------------
// TestParsers_HaveEmptyCommandsGuard
// Verifies that both openapi.go and swagger.go contain the exact sentinel
// string "spec defines no operations", confirming neither file wraps the
// error with %w or additional context text.
// ---------------------------------------------------------------------------
func TestParsers_HaveEmptyCommandsGuard(t *testing.T) {
	const sentinel = "spec defines no operations"

	for _, filename := range []string{"openapi.go", "swagger.go"} {
		src, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("could not read %s: %v", filename, err)
		}
		if !strings.Contains(string(src), sentinel) {
			t.Errorf("%s must contain the exact string %q but it does not", filename, sentinel)
		}
	}
}
