package parser

import (
	"testing"
)

// findParam returns the first param with the given name across an operation's
// query and body params, or nil.
func findParam(ps []ParamData, name string) *ParamData {
	for i := range ps {
		if ps[i].Name == name {
			return &ps[i]
		}
	}
	return nil
}

func firstOp(t *testing.T, cli *CLIData) OperationData {
	t.Helper()
	if len(cli.Commands) == 0 || len(cli.Commands[0].Operations) == 0 {
		t.Fatal("no operations parsed")
	}
	return cli.Commands[0].Operations[0]
}

const refSpec = `
openapi: 3.0.0
info:
  title: Ref API
servers:
  - url: https://a.example.com
    description: Prod
  - url: https://b.example.com
    description: Staging
components:
  parameters:
    PageParam:
      name: page
      in: query
      schema:
        type: integer
  schemas:
    Pet:
      type: object
      required: [name]
      properties:
        name: {type: string}
        age: {type: integer}
        neutered: {type: boolean}
        tags:
          type: array
          items: {type: string}
paths:
  /pets:
    get:
      operationId: listPets
      tags: [pets]
      parameters:
        - $ref: '#/components/parameters/PageParam'
    post:
      operationId: createPet
      tags: [pets]
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pet'
`

func TestOpenAPI_RefBodyResolved(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(refSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	// createPet is the second op (sorted: listPets before? operations are in
	// path/method order: GET then POST). Find the POST op by body params.
	var post OperationData
	for _, op := range cli.Commands[0].Operations {
		if len(op.BodyParams) > 0 {
			post = op
		}
	}
	if len(post.BodyParams) != 4 {
		t.Fatalf("expected 4 body params from $ref Pet, got %d", len(post.BodyParams))
	}
	if p := findParam(post.BodyParams, "name"); p == nil || !p.Required {
		t.Errorf("name should be a required body param: %+v", p)
	}
	if p := findParam(post.BodyParams, "neutered"); p == nil || p.GoType != "bool" {
		t.Errorf("neutered should be bool, got %+v", p)
	}
	if p := findParam(post.BodyParams, "tags"); p == nil || !p.IsArray || p.ElemType != "string" {
		t.Errorf("tags should be a string array, got %+v", p)
	}
	if p := findParam(post.BodyParams, "age"); p == nil || p.GoType != "int" {
		t.Errorf("age should be int, got %+v", p)
	}
}

func TestOpenAPI_ParameterRefResolved(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(refSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	var get OperationData
	for _, op := range cli.Commands[0].Operations {
		if op.Method == "GET" {
			get = op
		}
	}
	p := findParam(get.QueryParams, "page")
	if p == nil {
		t.Fatal("page query param (resolved from $ref) missing")
	}
	if p.GoType != "int" {
		t.Errorf("page should be int from resolved $ref, got %q", p.GoType)
	}
}

func TestOpenAPI_MultiServerParsed(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(refSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	if len(cli.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(cli.Servers))
	}
	if cli.ServerEnvVar != "REF_API__SERVER" {
		t.Errorf("ServerEnvVar = %q, want REF_API__SERVER", cli.ServerEnvVar)
	}
	if cli.Servers[0].URL != "https://a.example.com" || cli.Servers[0].Description != "Prod" {
		t.Errorf("server[0] = %+v", cli.Servers[0])
	}
}

func TestOpenAPI_HasArrayParamsFlag(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(refSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	if !cli.HasArrayParams {
		t.Error("HasArrayParams should be true when a spec has array params")
	}
}

const swaggerRefSpec = `{
  "swagger": "2.0",
  "info": {"title": "Widget API"},
  "host": "api.widget.io",
  "schemes": ["https"],
  "definitions": {
    "Widget": {
      "type": "object",
      "required": ["sku"],
      "properties": {
        "sku": {"type": "string"},
        "qty": {"type": "integer"},
        "tags": {"type": "array", "items": {"type": "string"}}
      }
    }
  },
  "paths": {
    "/widgets": {
      "post": {
        "operationId": "createWidget",
        "tags": ["widgets"],
        "parameters": [
          {"name": "body", "in": "body", "schema": {"$ref": "#/definitions/Widget"}}
        ]
      }
    }
  }
}`

func TestSwagger_RefBodyResolved(t *testing.T) {
	cli, err := ParseSwagger([]byte(swaggerRefSpec))
	if err != nil {
		t.Fatalf("ParseSwagger: %v", err)
	}
	op := firstOp(t, cli)
	if len(op.BodyParams) != 3 {
		t.Fatalf("expected 3 body params from $ref Widget, got %d", len(op.BodyParams))
	}
	if p := findParam(op.BodyParams, "sku"); p == nil || !p.Required {
		t.Errorf("sku should be required: %+v", p)
	}
	if p := findParam(op.BodyParams, "tags"); p == nil || !p.IsArray {
		t.Errorf("tags should be an array: %+v", p)
	}
}

// TestProto_DepthAwareParsing exercises maps, field options, nested messages,
// enums and block comments — cases the old line-regex parser mishandled.
func TestProto_DepthAwareParsing(t *testing.T) {
	proto := []byte(`// @server: api.example.com:443
syntax = "proto3";
package com.acme.v1;

/* block { comment } with ; punctuation */
enum Status { STATUS_UNKNOWN = 0; ACTIVE = 1; }

message CreateRequest {
  string name = 1;
  bool   active = 2;
  int32  retries = 3 [deprecated = true];
  map<string, string> labels = 4;
  Status status = 5;
  message Inner { string note = 1; }
  Inner inner = 6;
  repeated string tags = 7;
}
message CreateReply { string id = 1; }
service Widgets {
  rpc Create (CreateRequest) returns (CreateReply);
}
`)
	cli, err := ParseProto(proto, "complex.proto")
	if err != nil {
		t.Fatalf("ParseProto: %v", err)
	}
	op := firstOp(t, cli)
	got := map[string]string{} // name -> GoType
	for _, p := range op.InputParams {
		got[p.Name] = p.GoType
	}
	want := map[string]string{
		"name": "string", "active": "bool", "retries": "int",
		"labels": "string" /* map */, "status": "string" /* enum */, "inner": "string" /* message */, "tags": "string", /* repeated */
	}
	if len(got) != len(want) {
		t.Fatalf("got %d input params %v, want %d", len(got), got, len(want))
	}
	for name, typ := range want {
		if got[name] != typ {
			t.Errorf("param %q GoType = %q, want %q", name, got[name], typ)
		}
	}
	// Nested message Inner should also have been recorded for completeness.
	if op.InputParams[0].Name != "name" {
		t.Errorf("field order not preserved: first = %q", op.InputParams[0].Name)
	}
}
