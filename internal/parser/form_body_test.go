package parser

import (
	"testing"
)

// formSpec mirrors the shape of Zulip's API: form-urlencoded bodies whose
// non-scalar fields are declared JSON-encoded via the media type's encoding
// map, plus a requestBody $ref and a multipart file upload.
const formSpec = `
openapi: 3.0.1
info:
  title: Chat API
servers:
  - url: https://chat.example.com/api/v1
components:
  schemas:
    Content:
      type: string
  requestBodies:
    UpdateUser:
      content:
        application/x-www-form-urlencoded:
          schema:
            type: object
            properties:
              full_name: {type: string}
paths:
  /messages:
    post:
      operationId: send-message
      tags: [messages]
      requestBody:
        required: true
        content:
          application/x-www-form-urlencoded:
            schema:
              type: object
              required: [type, to, content]
              properties:
                type: {type: string}
                to:
                  oneOf:
                    - type: string
                    - type: array
                      items: {type: integer}
                content:
                  $ref: '#/components/schemas/Content'
                read_by_sender: {type: boolean}
                user_ids:
                  type: array
                  items: {type: integer}
                tags:
                  type: array
                  items: {type: string}
            encoding:
              to:
                contentType: application/json
              read_by_sender:
                contentType: application/json
              user_ids:
                contentType: application/json
  /users/{user_id}:
    patch:
      operationId: update-user
      tags: [users]
      parameters:
        - name: user_id
          in: path
          required: true
          schema: {type: integer}
      requestBody:
        $ref: '#/components/requestBodies/UpdateUser'
  /upload:
    post:
      operationId: upload-file
      tags: [uploads]
      requestBody:
        content:
          multipart/form-data:
            schema:
              type: object
              properties:
                filename:
                  type: string
                  format: binary
  /both:
    post:
      operationId: both
      tags: [z]
      requestBody:
        content:
          application/x-www-form-urlencoded:
            schema:
              type: object
              properties:
                form_field: {type: string}
          application/json:
            schema:
              type: object
              properties:
                json_field: {type: string}
`

func opByName(t *testing.T, cli *CLIData, name string) OperationData {
	t.Helper()
	for _, c := range cli.Commands {
		for _, op := range c.Operations {
			if op.Name == name {
				return op
			}
		}
	}
	t.Fatalf("operation %q not found", name)
	return OperationData{}
}

func TestOpenAPI_FormBody_BecomesFlags(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(formSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	op := opByName(t, cli, "send-message")
	if !op.FormBody {
		t.Error("form-urlencoded requestBody must set FormBody")
	}
	for _, name := range []string{"type", "to", "content", "read_by_sender", "user_ids", "tags"} {
		if findParam(op.BodyParams, name) == nil {
			t.Errorf("form field %q must become a body param", name)
		}
	}
	for _, name := range []string{"type", "to", "content"} {
		if p := findParam(op.BodyParams, name); p != nil && !p.Required {
			t.Errorf("form field %q must be required", name)
		}
	}
	if p := findParam(op.BodyParams, "to"); p != nil && (p.GoType != "string" || p.IsArray) {
		t.Errorf("oneOf field must be a raw string flag, got GoType=%q IsArray=%v", p.GoType, p.IsArray)
	}
	if p := findParam(op.BodyParams, "user_ids"); p == nil || !p.IsArray || p.ElemType != "int" || !p.FormJSON {
		t.Errorf("JSON-encoded int array must be IsArray/ElemType int/FormJSON, got %+v", p)
	}
	if p := findParam(op.BodyParams, "tags"); p == nil || !p.IsArray || p.FormJSON {
		t.Errorf("array without encoding entry must not be FormJSON, got %+v", p)
	}
	if !cli.HasArrayParams {
		t.Error("form array fields must set HasArrayParams")
	}
}

func TestOpenAPI_FormBody_RequestBodyRefResolved(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(formSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	op := opByName(t, cli, "update-user")
	if !op.FormBody || findParam(op.BodyParams, "full_name") == nil {
		t.Errorf("components/requestBodies $ref must resolve to form body params, got FormBody=%v params=%+v", op.FormBody, op.BodyParams)
	}
}

func TestOpenAPI_MultipartBody_NotExpanded(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(formSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	op := opByName(t, cli, "upload-file")
	if len(op.BodyParams) != 0 || op.FormBody {
		t.Errorf("multipart/form-data is unsupported and must not produce body params, got FormBody=%v params=%+v", op.FormBody, op.BodyParams)
	}
}

func TestOpenAPI_JSONPreferredOverForm(t *testing.T) {
	cli, err := ParseOpenAPI([]byte(formSpec))
	if err != nil {
		t.Fatalf("ParseOpenAPI: %v", err)
	}
	op := opByName(t, cli, "both")
	if op.FormBody {
		t.Error("application/json must be preferred when both media types are offered")
	}
	if findParam(op.BodyParams, "json_field") == nil || findParam(op.BodyParams, "form_field") != nil {
		t.Errorf("expected only the JSON schema's fields, got %+v", op.BodyParams)
	}
	if !cli.HasBodyParams {
		t.Error("a JSON body must still set HasBodyParams")
	}
}

const swaggerFormSpec = `
swagger: "2.0"
info:
  title: Form API
host: api.example.com
schemes: [https]
paths:
  /tokens:
    post:
      operationId: createToken
      tags: [auth]
      consumes: [application/x-www-form-urlencoded]
      parameters:
        - name: username
          in: formData
          type: string
          required: true
        - name: ttl
          in: formData
          type: integer
        - name: scopes
          in: formData
          type: array
          items: {type: string}
          collectionFormat: multi
        - name: roles
          in: formData
          type: array
          items: {type: string}
  /avatar:
    post:
      operationId: uploadAvatar
      tags: [files]
      consumes: [multipart/form-data]
      parameters:
        - name: caption
          in: formData
          type: string
        - name: file
          in: formData
          type: file
`

func TestSwagger_FormDataBecomesFormBody(t *testing.T) {
	cli, err := ParseSwagger([]byte(swaggerFormSpec))
	if err != nil {
		t.Fatalf("ParseSwagger: %v", err)
	}
	op := opByName(t, cli, "createtoken")
	if !op.FormBody {
		t.Error("formData params must produce a form body")
	}
	if p := findParam(op.BodyParams, "username"); p == nil || !p.Required {
		t.Errorf("required formData string must be a required body param, got %+v", p)
	}
	if p := findParam(op.BodyParams, "ttl"); p == nil || p.GoType != "int" {
		t.Errorf("integer formData must be an int flag, got %+v", p)
	}
	if p := findParam(op.BodyParams, "scopes"); p == nil || !p.IsArray {
		t.Errorf("collectionFormat multi array must be IsArray (repeated keys), got %+v", p)
	}
	if p := findParam(op.BodyParams, "roles"); p == nil || p.IsArray || p.GoType != "string" {
		t.Errorf("csv (default) array must be passed through as one string value, got %+v", p)
	}
	if cli.HasBodyParams {
		t.Error("formData must not set HasBodyParams (JSON-only)")
	}
}

func TestSwagger_FormDataFileUpload_Skipped(t *testing.T) {
	cli, err := ParseSwagger([]byte(swaggerFormSpec))
	if err != nil {
		t.Fatalf("ParseSwagger: %v", err)
	}
	op := opByName(t, cli, "uploadavatar")
	if op.FormBody || len(op.BodyParams) != 0 {
		t.Errorf("an op with a type: file formData param needs multipart and must be skipped, got FormBody=%v params=%+v", op.FormBody, op.BodyParams)
	}
}
