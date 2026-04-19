package parser

import (
	"strings"
	"testing"
)

// ---- operationName / methodPathSlug tests ----

// TestOperationName_NoOperationID_UsesMethodPathSlug verifies that GET /users/{id}
// with no operationId resolves to "get-users-id".
func TestOperationName_NoOperationID_UsesMethodPathSlug(t *testing.T) {
	pop := pathOp{
		method: "GET",
		path:   "/users/{id}",
		op:     &oaOperation{},
	}
	got := operationName(pop)
	want := "get-users-id"
	if got != want {
		t.Errorf("operationName = %q, want %q", got, want)
	}
}

// TestOperationName_EmptyOperationID_FallsBackToSlug verifies that operationId "---"
// (normalizes to empty) falls back to method+path slug.
func TestOperationName_EmptyOperationID_FallsBackToSlug(t *testing.T) {
	pop := pathOp{
		method: "POST",
		path:   "/items",
		op:     &oaOperation{OperationID: "---"},
	}
	got := operationName(pop)
	// Should NOT be empty and should start with "post"
	if got == "" {
		t.Error("operationName must not be empty when operationId normalizes to empty")
	}
	if !strings.HasPrefix(got, "post") {
		t.Errorf("expected operationName to start with 'post', got %q", got)
	}
}

// TestOperationName_ValidOperationID_UsesIt verifies that a valid operationId
// is used as-is (after ToCommandName normalization, no camelCase splitting).
func TestOperationName_ValidOperationID_UsesIt(t *testing.T) {
	pop := pathOp{
		method: "GET",
		path:   "/users/{id}",
		op:     &oaOperation{OperationID: "get-user"},
	}
	got := operationName(pop)
	want := "get-user"
	if got != want {
		t.Errorf("operationName = %q, want %q", got, want)
	}
}

// TestToCommandName_EmptyNormalization verifies that toCommandName returns a
// non-nil error when the input normalizes to an empty string.
func TestToCommandName_EmptyNormalization(t *testing.T) {
	_, err := toCommandName("---")
	if err == nil {
		t.Error("toCommandName('---') must return a non-nil error")
	}
}

// TestToCommandName_ValidInput verifies that toCommandName succeeds for valid input.
func TestToCommandName_ValidInput(t *testing.T) {
	name, err := toCommandName("getUsers")
	if err != nil {
		t.Fatalf("toCommandName('getUsers') returned unexpected error: %v", err)
	}
	if name == "" {
		t.Error("toCommandName('getUsers') must return a non-empty name")
	}
}

// TestToCommandName_DoesNotChangeExportedSignature verifies that the exported
// ToCommandName signature is unchanged (no error return) by calling it directly.
func TestToCommandName_DoesNotChangeExportedSignature(t *testing.T) {
	// This is a compile-time check: if the signature changed, this would not compile.
	result := ToCommandName("hello world")
	if result != "hello-world" {
		t.Errorf("ToCommandName('hello world') = %q, want 'hello-world'", result)
	}
}

// ---- OpenAPI 3.x dedup tests ----

// TestBuildCommandsFromOps_NoDuplicateNames verifies that two GET operations
// on different paths under the same tag produce distinct non-empty Names.
func TestBuildCommandsFromOps_NoDuplicateNames(t *testing.T) {
	spec := []byte(`openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /users:
    get:
      tags:
        - users
      summary: List users
  /users/{id}:
    get:
      tags:
        - users
      summary: Get user by id
`)
	cli, err := ParseOpenAPI(spec)
	if err != nil {
		t.Fatalf("ParseOpenAPI returned error: %v", err)
	}
	var usersCmd *CommandData
	for i := range cli.Commands {
		if cli.Commands[i].Name == "users" {
			usersCmd = &cli.Commands[i]
			break
		}
	}
	if usersCmd == nil {
		t.Fatal("expected a 'users' command")
	}
	if len(usersCmd.Operations) < 2 {
		t.Fatalf("expected at least 2 operations, got %d", len(usersCmd.Operations))
	}

	seen := map[string]bool{}
	for _, op := range usersCmd.Operations {
		if op.Name == "" {
			t.Errorf("operation has empty Name")
		}
		if seen[op.Name] {
			t.Errorf("duplicate operation Name %q within 'users' command", op.Name)
		}
		seen[op.Name] = true
	}
}

// TestBuildCommandsFromOps_SameNameDifferentTags_NoCrossTagCollision verifies
// that operations in different tags may share the same operation name without
// being incorrectly deduplicated (seenNames must be per-tag, not global).
func TestBuildCommandsFromOps_SameNameDifferentTags_NoCrossTagCollision(t *testing.T) {
	spec := []byte(`openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /users:
    get:
      tags:
        - users
      operationId: list
      summary: List users
  /items:
    get:
      tags:
        - items
      operationId: list
      summary: List items
`)
	cli, err := ParseOpenAPI(spec)
	if err != nil {
		t.Fatalf("ParseOpenAPI returned error: %v", err)
	}

	// Each tag should have exactly one operation named "list" (no cross-tag dedup).
	for _, cmd := range cli.Commands {
		if len(cmd.Operations) != 1 {
			t.Errorf("command %q: expected 1 operation, got %d", cmd.Name, len(cmd.Operations))
			continue
		}
		if cmd.Operations[0].Name != "list" {
			t.Errorf("command %q: expected operation Name='list', got %q", cmd.Name, cmd.Operations[0].Name)
		}
	}
}

// ---- Swagger 2.0 dedup tests ----

// TestBuildCommandsFromSwagger_NoDuplicateNames verifies that two GET operations
// under the same tag in a Swagger 2.0 spec produce distinct Names.
func TestBuildCommandsFromSwagger_NoDuplicateNames(t *testing.T) {
	spec := []byte(`swagger: "2.0"
info:
  title: Test API
  version: "1.0"
host: api.example.com
basePath: /v1
schemes:
  - https
paths:
  /users:
    get:
      tags:
        - users
      summary: List users
  /users/{id}:
    get:
      tags:
        - users
      summary: Get user
`)
	cli, err := ParseSwagger(spec)
	if err != nil {
		t.Fatalf("ParseSwagger returned error: %v", err)
	}
	var usersCmd *CommandData
	for i := range cli.Commands {
		if cli.Commands[i].Name == "users" {
			usersCmd = &cli.Commands[i]
			break
		}
	}
	if usersCmd == nil {
		t.Fatal("expected a 'users' command")
	}
	if len(usersCmd.Operations) < 2 {
		t.Fatalf("expected at least 2 operations, got %d", len(usersCmd.Operations))
	}

	seen := map[string]bool{}
	for _, op := range usersCmd.Operations {
		if op.Name == "" {
			t.Errorf("operation has empty Name")
		}
		if seen[op.Name] {
			t.Errorf("duplicate operation Name %q within 'users' command (Swagger)", op.Name)
		}
		seen[op.Name] = true
	}
}

// ---- gRPC dedup tests ----

// TestParseProto_GRPCDedupCollidingRPCNames verifies that two RPCs whose names
// normalize to the same string (after ToCommandName) produce distinct operation Names.
func TestParseProto_GRPCDedupCollidingRPCNames(t *testing.T) {
	// "GetUser" and "get_user" both normalize to "get-user" via ToCommandName.
	proto := []byte(`syntax = "proto3";
service UserService {
  rpc GetUser (UserRequest) returns (UserReply);
  rpc get_user (UserRequest) returns (UserReply);
}
message UserRequest { string id = 1; }
message UserReply { string name = 1; }
`)
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	if len(cli.Commands) == 0 {
		t.Fatal("expected at least one command")
	}
	cmd := cli.Commands[0]
	if len(cmd.Operations) < 2 {
		t.Fatalf("expected at least 2 operations, got %d", len(cmd.Operations))
	}

	seen := map[string]bool{}
	for _, op := range cmd.Operations {
		if op.Name == "" {
			t.Errorf("gRPC operation has empty Name")
		}
		if seen[op.Name] {
			t.Errorf("duplicate gRPC operation Name %q within service", op.Name)
		}
		seen[op.Name] = true
	}
}

// TestParseProto_GRPCEmptyNameFallback verifies that an RPC whose name normalizes
// to empty gets a stable "rpc-N" fallback (not an empty string).
func TestParseProto_GRPCEmptyNameFallback(t *testing.T) {
	// RPC named "___" normalizes to "" via ToCommandName.
	proto := []byte(`syntax = "proto3";
service FooService {
  rpc ___ (FooRequest) returns (FooReply);
}
message FooRequest { string x = 1; }
message FooReply { string y = 1; }
`)
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	if len(cli.Commands) == 0 || len(cli.Commands[0].Operations) == 0 {
		t.Fatal("expected at least one command/operation")
	}
	op := cli.Commands[0].Operations[0]
	if op.Name == "" {
		t.Error("gRPC operation Name must not be empty when RPC name normalizes to empty")
	}
	if !strings.HasPrefix(op.Name, "rpc-") {
		t.Errorf("expected Name to start with 'rpc-', got %q", op.Name)
	}
}
