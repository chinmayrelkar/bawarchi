package parser

import (
	"os"
	"strings"
	"testing"
)

// minimalSwagger2SpecYAML returns a Swagger 2.0 YAML spec with a single operation
// that has one path/query parameter of the given type.
func minimalSwagger2SpecYAML(paramIn, paramType string) []byte {
	return []byte(`swagger: "2.0"
info:
  title: Test API
  version: "1.0"
host: api.example.com
basePath: /v1
schemes:
  - https
paths:
  /items/{id}:
    get:
      tags:
        - items
      summary: Get item
      operationId: getItem
      parameters:
        - name: id
          in: ` + paramIn + `
          required: true
          type: ` + paramType + `
`)
}

// minimalOpenAPI3SpecYAML returns an OpenAPI 3.x YAML spec with a single integer param.
func minimalOpenAPI3SpecYAML() []byte {
	return []byte(`openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: https://api.example.com/v1
paths:
  /items/{id}:
    get:
      tags:
        - items
      summary: Get item
      operationId: getItem
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
`)
}

// TestSwagger2Param_Integer confirms that a Swagger 2.0 integer path param
// produces FlagFunc=IntVar and GoType=int.
func TestSwagger2Param_Integer(t *testing.T) {
	data := minimalSwagger2SpecYAML("path", "integer")
	cli, err := ParseSwagger(data)
	if err != nil {
		t.Fatalf("ParseSwagger returned error: %v", err)
	}
	param := findFirstPathParam(t, cli)
	if param.FlagFunc != "IntVar" {
		t.Errorf("expected FlagFunc=IntVar for integer param, got %q", param.FlagFunc)
	}
	if param.GoType != "int" {
		t.Errorf("expected GoType=int for integer param, got %q", param.GoType)
	}
}

// TestSwagger2Param_Number confirms that a Swagger 2.0 number query param
// produces FlagFunc=Float64Var and GoType=float64.
func TestSwagger2Param_Number(t *testing.T) {
	data := minimalSwagger2SpecYAML("query", "number")
	cli, err := ParseSwagger(data)
	if err != nil {
		t.Fatalf("ParseSwagger returned error: %v", err)
	}
	param := findFirstQueryParam(t, cli)
	if param.FlagFunc != "Float64Var" {
		t.Errorf("expected FlagFunc=Float64Var for number param, got %q", param.FlagFunc)
	}
	if param.GoType != "float64" {
		t.Errorf("expected GoType=float64 for number param, got %q", param.GoType)
	}
}

// TestSwagger2Param_NoType_DefaultsToString tests the firstNonEmpty fallback to "string"
// when both raw.Type and raw.Schema.Type are empty (no type field in the YAML at all).
func TestSwagger2Param_NoType_DefaultsToString(t *testing.T) {
	// Build YAML without any type: line so both raw.Type and raw.Schema.Type are empty.
	data := []byte(`swagger: "2.0"
info:
  title: Test API
  version: "1.0"
host: api.example.com
basePath: /v1
schemes:
  - https
paths:
  /search:
    get:
      tags:
        - search
      summary: Search
      operationId: search
      parameters:
        - name: q
          in: query
          required: false
`)
	cli, err := ParseSwagger(data)
	if err != nil {
		t.Fatalf("ParseSwagger returned error: %v", err)
	}
	param := findFirstQueryParam(t, cli)
	if param.FlagFunc != "StringVar" {
		t.Errorf("expected FlagFunc=StringVar when type is absent, got %q", param.FlagFunc)
	}
	if param.GoType != "string" {
		t.Errorf("expected GoType=string when type is absent, got %q", param.GoType)
	}
}

// TestOpenAPI3Param_SchemaType_Unaffected confirms that an OpenAPI 3.x integer
// path param (type inside schema) still resolves to IntVar after the fix.
func TestOpenAPI3Param_SchemaType_Unaffected(t *testing.T) {
	data := minimalOpenAPI3SpecYAML()
	cli, err := ParseOpenAPI(data)
	if err != nil {
		t.Fatalf("ParseOpenAPI returned error: %v", err)
	}
	param := findFirstPathParam(t, cli)
	if param.FlagFunc != "IntVar" {
		t.Errorf("expected FlagFunc=IntVar for OpenAPI 3.x integer param, got %q", param.FlagFunc)
	}
	if param.GoType != "int" {
		t.Errorf("expected GoType=int for OpenAPI 3.x integer param, got %q", param.GoType)
	}
}

// TestSwagger2DeadCode_Removed fails if either dead identifier reappears in swagger.go.
func TestSwagger2DeadCode_Removed(t *testing.T) {
	src, err := os.ReadFile("swagger.go")
	if err != nil {
		t.Fatalf("could not read swagger.go: %v", err)
	}
	content := string(src)
	if strings.Contains(content, "swagger2Parameter") {
		t.Error("swagger.go still contains 'swagger2Parameter' — it must be removed")
	}
	if strings.Contains(content, "swagger2ParamType") {
		t.Error("swagger.go still contains 'swagger2ParamType' — it must be removed")
	}
}

// TestOpenAPI3Closure_UsesSchemaType confirms ParseOpenAPI closure uses p.Schema.Type not p.Type.
func TestOpenAPI3Closure_UsesSchemaType(t *testing.T) {
	src, err := os.ReadFile("openapi.go")
	if err != nil {
		t.Fatalf("could not read openapi.go: %v", err)
	}
	content := string(src)
	// The closure passed to buildCommandsFromOps must reference p.Schema.Type.
	if !strings.Contains(content, "p.Schema.Type") {
		t.Error("openapi.go closure must return p.Schema.Type for OpenAPI 3.x type resolution")
	}
}

// TestOaParameter_HasTypeAndFormat confirms oaParameter has exported Type and Format fields.
func TestOaParameter_HasTypeAndFormat(t *testing.T) {
	// Compile-time check: instantiate oaParameter with Type and Format set.
	p := oaParameter{
		Name:   "count",
		In:     "query",
		Type:   "integer",
		Format: "int32",
	}
	if p.Type != "integer" {
		t.Errorf("expected oaParameter.Type='integer', got %q", p.Type)
	}
	if p.Format != "int32" {
		t.Errorf("expected oaParameter.Format='int32', got %q", p.Format)
	}
}

// findFirstPathParam is a helper to retrieve the first path param from the first operation.
func findFirstPathParam(t *testing.T, cli *CLIData) ParamData {
	t.Helper()
	for _, cmd := range cli.Commands {
		for _, op := range cmd.Operations {
			if len(op.PathParams) > 0 {
				return op.PathParams[0]
			}
		}
	}
	t.Fatal("no path params found in CLIData")
	return ParamData{}
}

// findFirstQueryParam is a helper to retrieve the first query param from the first operation.
func findFirstQueryParam(t *testing.T, cli *CLIData) ParamData {
	t.Helper()
	for _, cmd := range cli.Commands {
		for _, op := range cmd.Operations {
			if len(op.QueryParams) > 0 {
				return op.QueryParams[0]
			}
		}
	}
	t.Fatal("no query params found in CLIData")
	return ParamData{}
}

// TestOpenAPI3_RequestBody_InlineProperties verifies that an OpenAPI 3.x operation
// with an inline application/json requestBody produces BodyParams on the operation
// and sets cli.HasBodyParams.
func TestOpenAPI3_RequestBody_InlineProperties(t *testing.T) {
	spec := []byte(`openapi: "3.0.0"
info:
  title: Pet Store
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /pets:
    post:
      tags: [pets]
      operationId: createPet
      summary: Create a pet
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                  description: Pet name
                age:
                  type: integer
                  description: Pet age
      responses:
        "201":
          description: created
`)
	cli, err := ParseOpenAPI(spec)
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}

	if !cli.HasBodyParams {
		t.Error("cli.HasBodyParams must be true when a requestBody with properties is present")
	}
	op := cli.Commands[0].Operations[0]
	if len(op.BodyParams) != 2 {
		t.Fatalf("expected 2 body params, got %d: %+v", len(op.BodyParams), op.BodyParams)
	}
	// Properties must be sorted alphabetically (age before name).
	if op.BodyParams[0].Name != "age" {
		t.Errorf("BodyParams[0].Name = %q, want %q", op.BodyParams[0].Name, "age")
	}
	if op.BodyParams[0].GoType != "int" {
		t.Errorf("BodyParams[0].GoType = %q, want %q", op.BodyParams[0].GoType, "int")
	}
	if op.BodyParams[1].Name != "name" {
		t.Errorf("BodyParams[1].Name = %q, want %q", op.BodyParams[1].Name, "name")
	}
	if op.BodyParams[1].GoType != "string" {
		t.Errorf("BodyParams[1].GoType = %q, want %q", op.BodyParams[1].GoType, "string")
	}
}

// TestSwagger2_BodyParam_InlineProperties verifies that a Swagger 2.0 operation
// with an in:body parameter and inline schema.properties produces BodyParams.
func TestSwagger2_BodyParam_InlineProperties(t *testing.T) {
	spec := []byte(`swagger: "2.0"
info:
  title: Pet Store
  version: "1.0"
host: api.example.com
basePath: /v1
schemes:
  - https
paths:
  /pets:
    post:
      tags:
        - pets
      operationId: createPet
      summary: Create a pet
      parameters:
        - name: body
          in: body
          required: true
          schema:
            type: object
            properties:
              name:
                type: string
                description: Pet name
              count:
                type: integer
                description: Pet count
`)
	cli, err := ParseSwagger(spec)
	if err != nil {
		t.Fatalf("ParseSwagger: %v", err)
	}

	if !cli.HasBodyParams {
		t.Error("cli.HasBodyParams must be true when an in:body parameter with properties is present")
	}
	op := cli.Commands[0].Operations[0]
	if len(op.BodyParams) != 2 {
		t.Fatalf("expected 2 body params, got %d: %+v", len(op.BodyParams), op.BodyParams)
	}
	// Properties must be sorted alphabetically (count before name).
	if op.BodyParams[0].Name != "count" {
		t.Errorf("BodyParams[0].Name = %q, want %q", op.BodyParams[0].Name, "count")
	}
	if op.BodyParams[1].Name != "name" {
		t.Errorf("BodyParams[1].Name = %q, want %q", op.BodyParams[1].Name, "name")
	}
}

// TestOpenAPI3_RequestBody_NoBodyParams_ForGetOp verifies that a GET operation
// without a requestBody has no BodyParams and HasBodyParams stays false.
func TestOpenAPI3_RequestBody_NoBodyParams_ForGetOp(t *testing.T) {
	cli, err := ParseOpenAPI(minimalOpenAPI3SpecYAML())
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	if cli.HasBodyParams {
		t.Error("HasBodyParams must be false for a spec with only GET operations")
	}
	op := cli.Commands[0].Operations[0]
	if len(op.BodyParams) != 0 {
		t.Errorf("GET op must have 0 BodyParams, got %d", len(op.BodyParams))
	}
}
