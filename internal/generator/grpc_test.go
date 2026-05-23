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
		Name:          "testcli",
		Description:   "Test CLI",
		BaseURL:       "localhost:50051",
		BaseURLEnvVar: "TESTCLI__SERVER_ADDR",
		AuthEnvVar:    "TEST_API_KEY",
		Transport:     parser.TransportGRPC,
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

// TestGenerateGRPC_PlaintextVarDecl verifies that serverAddr is declared as a var, not a const.
func TestGenerateGRPC_PlaintextVarDecl(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	if !strings.Contains(got, "var serverAddr =") {
		t.Errorf("expected 'var serverAddr =' in generated source, got:\n%s", got)
	}
	if strings.Contains(got, "const serverAddr") {
		t.Errorf("serverAddr must not be a const in generated source")
	}
}

// TestGenerateGRPC_PlaintextInitBlock verifies the init() function is present and
// references the BaseURLEnvVar template variable.
func TestGenerateGRPC_PlaintextInitBlock(t *testing.T) {
	data := minimalGRPCData()
	data.BaseURLEnvVar = "TESTCLI__SERVER_ADDR"

	src, err := generateGRPC(data)
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	if !strings.Contains(got, "func init()") {
		t.Errorf("expected 'func init()' in generated source")
	}
	if !strings.Contains(got, `os.Getenv("TESTCLI__SERVER_ADDR")`) {
		t.Errorf("expected os.Getenv(\"TESTCLI__SERVER_ADDR\") in init block; got:\n%s", got)
	}
	if !strings.Contains(got, "serverAddr = v") {
		t.Errorf("expected 'serverAddr = v' assignment in init block")
	}
}

// TestGenerateGRPC_PlaintextSignature verifies grpcCall accepts a plaintext bool parameter.
func TestGenerateGRPC_PlaintextSignature(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	if !strings.Contains(got, "func grpcCall(service, method string, fields map[string]interface{}, plaintext bool)") {
		t.Errorf("grpcCall must accept 'plaintext bool' parameter and use map[string]interface{}; got source:\n%s", got)
	}
}

// TestGenerateGRPC_PlaintextAuthGuard verifies the security guard that prevents
// sending bearer tokens over plaintext connections.
func TestGenerateGRPC_PlaintextAuthGuard(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	if !strings.Contains(got, "cleartext") {
		t.Errorf("expected plaintext+auth guard message (mentioning cleartext) in generated source")
	}
	if !strings.Contains(got, "os.Exit(1)") {
		t.Errorf("expected os.Exit(1) in grpcCall body for guard")
	}

	// Guard must appear BEFORE the exec.Command("grpcurl", ...) call
	guardIdx := strings.Index(got, "plaintext {")
	execIdx := strings.Index(got, `exec.Command("grpcurl"`)
	if guardIdx == -1 {
		t.Errorf("expected plaintext guard block in generated source")
	}
	if execIdx == -1 {
		t.Errorf("expected exec.Command(\"grpcurl\") in generated source")
	}
	if guardIdx != -1 && execIdx != -1 && guardIdx > execIdx {
		t.Errorf("plaintext+auth guard must appear before exec.Command line")
	}
}

// TestGenerateGRPC_PlaintextConditionalAppend verifies -plaintext is only appended
// conditionally (not hardcoded unconditionally).
func TestGenerateGRPC_PlaintextConditionalAppend(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	if strings.Contains(got, `[]string{"-plaintext"}`) {
		t.Errorf("generated source must not contain hardcoded []string{\"-plaintext\"}")
	}
	if !strings.Contains(got, `append(args, "-plaintext")`) {
		t.Errorf("expected conditional append(args, \"-plaintext\") in generated source")
	}
}

// TestGenerateGRPC_InitOverridesServerAddr verifies the init() block renders the
// BaseURLEnvVar substitution so os.Getenv reads the correct env var name.
func TestGenerateGRPC_InitOverridesServerAddr(t *testing.T) {
	data := minimalGRPCData()
	data.BaseURLEnvVar = "TESTCLI__BASE_URL"

	src, err := generateGRPC(data)
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	want := `os.Getenv("TESTCLI__BASE_URL")`
	if !strings.Contains(got, want) {
		t.Errorf("expected %q in generated source; got:\n%s", want, got)
	}
}

// TestGenerateGRPC_NativeTypeMap verifies that the fields map uses interface{}
// so json.Marshal emits native JSON types (42 not "42", true not "true").
func TestGenerateGRPC_NativeTypeMap(t *testing.T) {
	src, err := generateGRPC(minimalGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	if !strings.Contains(got, "map[string]interface{}{}") {
		t.Error("fields must be map[string]interface{}{} to emit native JSON types")
	}
	if strings.Contains(got, "map[string]string{}") {
		t.Error("fields must not be map[string]string{} (would quote all values)")
	}
	if strings.Contains(got, "fmt.Sprintf") {
		t.Error("generated code must not use fmt.Sprintf to stringify field values")
	}
}

// typedGRPCData returns CLIData with int, float, and bool fields to test native type emission.
func typedGRPCData() *parser.CLIData {
	d := minimalGRPCData()
	d.Commands[0].Operations[0].InputParams = []parser.ParamData{
		{
			Name: "count", GoVarName: "Count", FlagName: "count",
			GoType: "int", FlagFunc: "IntVar", DefaultLiteral: "0", DefaultCmp: "!= 0",
		},
		{
			Name: "score", GoVarName: "Score", FlagName: "score",
			GoType: "float64", FlagFunc: "Float64Var", DefaultLiteral: "0.0", DefaultCmp: "!= 0.0",
		},
		{
			Name: "enabled", GoVarName: "Enabled", FlagName: "enabled",
			GoType: "bool", FlagFunc: "BoolVar", DefaultLiteral: "false", DefaultCmp: "!= false",
		},
	}
	return d
}

// TestGenerateGRPC_TypedFieldsNativeAssignment verifies that int/float/bool fields
// are assigned directly to the interface{} map without fmt.Sprintf wrapping.
func TestGenerateGRPC_TypedFieldsNativeAssignment(t *testing.T) {
	src, err := generateGRPC(typedGRPCData())
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}
	got := string(src)

	// Each param must be assigned directly, not via fmt.Sprintf
	for _, param := range []string{"pCount", "pScore", "pEnabled"} {
		direct := `fields["` + strings.ToLower(param[1:]) + `"] = ` + param
		if !strings.Contains(got, direct) {
			t.Errorf("expected direct assignment %q in generated source", direct)
		}
	}
	if strings.Contains(got, "fmt.Sprintf") {
		t.Error("generated code must not use fmt.Sprintf to stringify typed fields")
	}
}
