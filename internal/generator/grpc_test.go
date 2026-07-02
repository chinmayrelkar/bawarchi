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

// TestGRPCTemplateServiceHintInPrintUsage verifies that the generated source
// unconditionally includes the @service annotation hint in the printUsage block.
func TestGRPCTemplateServiceHintInPrintUsage(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	if !strings.Contains(code, "@service") {
		t.Error("generated printUsage must contain '@service' annotation hint")
	}
	// Ensure the hint is inside the printUsage function body
	printUsageIdx := strings.Index(code, "func printUsage()")
	if printUsageIdx == -1 {
		t.Fatal("generated code must contain 'func printUsage()'")
	}
	afterPrintUsage := code[printUsageIdx:]
	if !strings.Contains(afterPrintUsage, "@service") {
		t.Error("@service hint must appear inside the printUsage function body")
	}
}

// noAuthGRPCData returns CLIData with AuthEnvVar="" (simulating @noauth proto).
func noAuthGRPCData() *parser.CLIData {
	d := minimalGRPCData()
	d.AuthEnvVar = ""
	return d
}

// TestGRPCTemplate_NoAuth_NoAuthStrings verifies that when AuthEnvVar is empty
// the generated code contains none of the auth-specific strings.
func TestGRPCTemplate_NoAuth_NoAuthStrings(t *testing.T) {
	src, err := generateGRPC(noAuthGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	if strings.Contains(code, "Authorization: Bearer") {
		t.Error("noauth generated code must not contain 'Authorization: Bearer'")
	}
	if strings.Contains(code, "is not set") {
		t.Error("noauth generated code must not contain 'is not set'")
	}
	if strings.Contains(code, "os.Getenv(authEnvVar)") {
		t.Error("noauth generated code must not contain 'os.Getenv(authEnvVar)'")
	}
}

// TestGRPCTemplate_NoAuth_NoAuthEnvVarConst verifies that when AuthEnvVar is
// empty the authEnvVar const is not declared (preventing unused-identifier errors).
func TestGRPCTemplate_NoAuth_NoAuthEnvVarConst(t *testing.T) {
	src, err := generateGRPC(noAuthGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	if strings.Contains(code, "authEnvVar") {
		t.Error("noauth generated code must not declare or reference authEnvVar")
	}
}

// TestGRPCTemplate_WithAuth_AllAuthStrings verifies that when AuthEnvVar is set
// the generated code contains all three auth-specific strings.
func TestGRPCTemplate_WithAuth_AllAuthStrings(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	if !strings.Contains(code, "Authorization: Bearer") {
		t.Error("auth generated code must contain 'Authorization: Bearer'")
	}
	if !strings.Contains(code, "is not set") {
		t.Error("auth generated code must contain 'is not set'")
	}
	if !strings.Contains(code, "os.Getenv(authEnvVar)") {
		t.Error("auth generated code must contain 'os.Getenv(authEnvVar)'")
	}
}

// TestGRPCTemplateWithAuth_HasAuthBlock is an explicit regression test verifying
// that generateGRPC with a non-empty AuthEnvVar still produces auth-specific code.
func TestGRPCTemplateWithAuth_HasAuthBlock(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	if !strings.Contains(code, "Authorization: Bearer") {
		t.Error("auth path must contain 'Authorization: Bearer'")
	}
	if !strings.Contains(code, "is not set") {
		t.Error("auth path must contain 'is not set'")
	}
	if !strings.Contains(code, "os.Getenv(authEnvVar)") {
		t.Error("auth path must contain 'os.Getenv(authEnvVar)'")
	}
}

// TestGRPCTemplate_NoAuth_PrintUsage_HasNoauthNotRequired verifies that when
// AuthEnvVar is empty the printUsage block contains "@noauth" and does NOT
// contain "(required)".
func TestGRPCTemplate_NoAuth_PrintUsage_HasNoauthNotRequired(t *testing.T) {
	src, err := generateGRPC(noAuthGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	printUsageIdx := strings.Index(code, "func printUsage()")
	if printUsageIdx == -1 {
		t.Fatal("generated code must contain 'func printUsage()'")
	}
	afterPrintUsage := code[printUsageIdx:]

	if !strings.Contains(afterPrintUsage, "@noauth") {
		t.Error("printUsage must contain '@noauth' when AuthEnvVar is empty")
	}
	if strings.Contains(afterPrintUsage, "(required)") {
		t.Error("printUsage must NOT contain '(required)' when AuthEnvVar is empty")
	}
}

// TestGRPCTemplate_NoAuth_ServiceHintStillPresent verifies that even with
// @noauth the @service hint remains unconditionally inside printUsage.
func TestGRPCTemplate_NoAuth_ServiceHintStillPresent(t *testing.T) {
	src, err := generateGRPC(noAuthGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	code := string(src)

	printUsageIdx := strings.Index(code, "func printUsage()")
	if printUsageIdx == -1 {
		t.Fatal("generated code must contain 'func printUsage()'")
	}
	afterPrintUsage := code[printUsageIdx:]
	if !strings.Contains(afterPrintUsage, "@service") {
		t.Error("@service hint must appear inside printUsage even when @noauth is set")
	}
}
