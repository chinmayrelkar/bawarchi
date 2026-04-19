package parser

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// OpenAPI 3.x — error paths
// ---------------------------------------------------------------------------

// TestParseOpenAPI_NoPaths verifies that a spec with paths:{} returns a
// non-nil error containing "spec defines no operations".
func TestParseOpenAPI_NoPaths(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: Empty API
  version: "1.0"
paths: {}
`)
	_, err := ParseOpenAPI(spec)
	if err == nil {
		t.Fatal("expected non-nil error for spec with no paths, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// TestParseOpenAPI_PathItemNoOps verifies that a spec with a path item that
// has no HTTP operations returns a non-nil error containing
// "spec defines no operations".
func TestParseOpenAPI_PathItemNoOps(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: Empty API
  version: "1.0"
paths:
  /empty: {}
`)
	_, err := ParseOpenAPI(spec)
	if err == nil {
		t.Fatal("expected non-nil error for path item with no operations, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// OpenAPI 3.x — success paths
// ---------------------------------------------------------------------------

// TestParseOpenAPI_WithOperation verifies that a spec with at least one valid
// operation returns nil error and a non-empty Commands slice.
func TestParseOpenAPI_WithOperation(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: My API
  version: "1.0"
paths:
  /users:
    get:
      operationId: listUsers
      summary: List users
`)
	cli, err := ParseOpenAPI(spec)
	if err != nil {
		t.Fatalf("expected nil error for spec with an operation, got: %v", err)
	}
	if len(cli.Commands) == 0 {
		t.Fatal("expected non-empty Commands slice for spec with an operation")
	}
}

// ---------------------------------------------------------------------------
// Swagger 2.0 — error paths
// ---------------------------------------------------------------------------

// TestParseSwagger_NoPaths verifies that a Swagger 2.0 spec with paths:{}
// returns a non-nil error containing "spec defines no operations".
func TestParseSwagger_NoPaths(t *testing.T) {
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
		t.Fatal("expected non-nil error for swagger spec with no paths, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// TestParseSwagger_PathItemNoOps verifies that a Swagger 2.0 spec with a path
// item that has no HTTP operations returns a non-nil error containing
// "spec defines no operations".
func TestParseSwagger_PathItemNoOps(t *testing.T) {
	spec := []byte(`
swagger: "2.0"
info:
  title: Empty API
  version: "1.0"
host: api.example.com
schemes:
  - https
paths:
  /empty: {}
`)
	_, err := ParseSwagger(spec)
	if err == nil {
		t.Fatal("expected non-nil error for swagger path item with no operations, got nil")
	}
	if !strings.Contains(err.Error(), "spec defines no operations") {
		t.Fatalf("error must contain 'spec defines no operations', got: %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Swagger 2.0 — success paths
// ---------------------------------------------------------------------------

// TestParseSwagger_WithOperation verifies that a Swagger 2.0 spec with at
// least one valid operation returns nil error and a non-empty Commands slice.
func TestParseSwagger_WithOperation(t *testing.T) {
	spec := []byte(`
swagger: "2.0"
info:
  title: My API
  version: "1.0"
host: api.example.com
schemes:
  - https
paths:
  /users:
    get:
      operationId: listUsers
      summary: List users
`)
	cli, err := ParseSwagger(spec)
	if err != nil {
		t.Fatalf("expected nil error for swagger spec with an operation, got: %v", err)
	}
	if len(cli.Commands) == 0 {
		t.Fatal("expected non-empty Commands slice for swagger spec with an operation")
	}
}

// ---------------------------------------------------------------------------
// Extra OpenAPI 3.x success path (total = 7 new tests)
// ---------------------------------------------------------------------------

// TestParseOpenAPI_MultipleOperations verifies that multiple operations across
// multiple paths all result in a non-empty Commands slice.
func TestParseOpenAPI_MultipleOperations(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: Multi API
  version: "1.0"
paths:
  /users:
    get:
      operationId: listUsers
      summary: List users
    post:
      operationId: createUser
      summary: Create user
  /items:
    delete:
      operationId: deleteItem
      summary: Delete item
`)
	cli, err := ParseOpenAPI(spec)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(cli.Commands) == 0 {
		t.Fatal("expected non-empty Commands slice")
	}
}
