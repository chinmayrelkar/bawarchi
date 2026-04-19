package generator

import (
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

// minimalGRPCData returns a CLIData with one gRPC command/operation, used by
// multiple test cases.
func minimalGRPCData() *parser.CLIData {
	return &parser.CLIData{
		Name:        "testcli",
		Description: "Test CLI",
		BaseURL:     "localhost:50051",
		AuthEnvVar:  "TEST_API_KEY",
		Transport:   parser.TransportGRPC,
		Commands: []parser.CommandData{
			{
				Name:   "greet",
				GoName: "Greet",
				Operations: []parser.OperationData{
					{
						Name:        "say-hello",
						GoName:      "SayHello",
						GRPCService: "hello.Greeter",
						GRPCMethod:  "SayHello",
						InputParams: []parser.ParamData{
							{
								Name:           "name",
								GoVarName:      "Name",
								FlagName:       "name",
								GoType:         "string",
								FlagFunc:       "StringVar",
								DefaultLiteral: `""`,
								DefaultCmp:     `!= ""`,
								Description:    "Recipient name",
							},
						},
					},
				},
			},
		},
	}
}

// TestGRPCTemplateUsesJSONMarshal verifies that the generated source uses
// json.Marshal and does NOT contain the old manual-concat patterns.
func TestGRPCTemplateUsesJSONMarshal(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	// Must use json.Marshal
	if !strings.Contains(code, "json.Marshal(fields)") {
		t.Error("generated code must contain json.Marshal(fields)")
	}

	// Must import encoding/json
	if !strings.Contains(code, `"encoding/json"`) {
		t.Error(`generated code must import "encoding/json"`)
	}

	// Must NOT import strings
	if strings.Contains(code, `"strings"`) {
		t.Error(`generated code must NOT import "strings"`)
	}

	// Must NOT use old manual concat patterns
	if strings.Contains(code, "strings.Join") {
		t.Error("generated code must not contain strings.Join")
	}
	if strings.Contains(code, "fmt.Sprintf(\"%q: %q\"") {
		t.Error("generated code must not contain the old fmt.Sprintf key/value quoting pattern")
	}
	if strings.Contains(code, "var parts []string") {
		t.Error("generated code must not contain 'var parts []string'")
	}
}

// TestGRPCTemplateMarshalErrorHandled verifies that the generated source
// handles the json.Marshal error with os.Exit(1).
func TestGRPCTemplateMarshalErrorHandled(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	// Error must be checked and result in os.Exit(1)
	if !strings.Contains(code, "os.Exit(1)") {
		t.Error("generated code must call os.Exit(1) on marshal error")
	}
	// The error variable from json.Marshal must be checked
	if !strings.Contains(code, "if err != nil") {
		t.Error("generated code must check marshal error with 'if err != nil'")
	}
}

// TestGRPCTemplateBodyVariableName confirms the body variable name is 'body'
// (consistent with its use in args append).
func TestGRPCTemplateBodyVariableName(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	if !strings.Contains(code, "body := string(bodyBytes)") {
		t.Error(`generated code must contain 'body := string(bodyBytes)'`)
	}
	if !strings.Contains(code, `"-d", body`) {
		t.Error("generated code must pass body with \"-d\" flag to grpcurl")
	}
}
