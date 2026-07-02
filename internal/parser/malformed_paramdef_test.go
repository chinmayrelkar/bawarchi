package parser

import "testing"

// TestParseSwagger_MalformedParameterDefinition_SkippedNotFatal verifies that
// a Swagger 2.0 spec whose top-level `parameters` map contains a schema-shaped
// entry (an authoring mistake seen in real specs, e.g. Opsgenie's, where
// "required" is a []string instead of the bool a Parameter Object requires)
// is skipped with a warning rather than failing the entire parse.
func TestParseSwagger_MalformedParameterDefinition_SkippedNotFatal(t *testing.T) {
	spec := []byte(`swagger: "2.0"
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
          in: path
          required: true
          type: string
parameters:
  MalformedDef:
    type: object
    required:
      - targetIndex
    properties:
      targetIndex:
        type: integer
`)
	cli, err := ParseSwagger(spec)
	if err != nil {
		t.Fatalf("ParseSwagger returned error for spec with malformed parameter def: %v", err)
	}
	if len(cli.Commands) != 1 || len(cli.Commands[0].Operations) != 1 {
		t.Fatalf("expected the valid operation to still be parsed, got commands=%+v", cli.Commands)
	}
}
