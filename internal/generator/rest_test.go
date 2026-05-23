package generator

import (
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

// TestGenerateREST_NoDiscardedReadError verifies the generated doRequest no
// longer silently discards the error from io.ReadAll.
func TestGenerateREST_NoDiscardedReadError(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST returned error: %v", err)
	}

	src := string(out)

	// The old (buggy) pattern must be gone.
	if strings.Contains(src, "data, _ :=") {
		t.Errorf("generated code still silently discards io.ReadAll error via 'data, _ :='")
	}
}

// TestGenerateREST_VarDataDeclared verifies that `var data []byte` is present
// so `data` is properly introduced before the plain assignment.
func TestGenerateREST_VarDataDeclared(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST returned error: %v", err)
	}

	src := string(out)

	if !strings.Contains(src, "var data []byte") {
		t.Errorf("generated code does not declare 'var data []byte' before the io.ReadAll assignment")
	}
}

// TestGenerateREST_PlainAssignmentUsed verifies that `data, err =` (plain
// assignment, not short declaration) is used for io.ReadAll.
func TestGenerateREST_PlainAssignmentUsed(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST returned error: %v", err)
	}

	src := string(out)

	if !strings.Contains(src, "data, err = io.ReadAll(resp.Body)") {
		t.Errorf("generated code does not use plain assignment 'data, err = io.ReadAll(resp.Body)'")
	}
}

// TestGenerateREST_ReadErrorExitsToStderr verifies that the generated error
// handling writes to os.Stderr and calls os.Exit(1), consistent with all other
// error paths in doRequest.
func TestGenerateREST_ReadErrorExitsToStderr(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST returned error: %v", err)
	}

	src := string(out)

	if !strings.Contains(src, `fmt.Fprintf(os.Stderr, "error reading response: %v\n", err)`) {
		t.Errorf("generated code does not write io.ReadAll error to os.Stderr with expected message")
	}
	if !strings.Contains(src, "os.Exit(1)") {
		t.Errorf("generated code does not call os.Exit(1) after io.ReadAll error")
	}
}

// bodyRESTData returns a CLIData with a POST operation that has BodyParams,
// mirroring what the parser produces for a spec with a JSON requestBody.
func bodyRESTData() *parser.CLIData {
	d := minimalRESTData()
	d.HasBodyParams = true
	d.Commands[0].Operations = append(d.Commands[0].Operations, parser.OperationData{
		Name:   "create",
		GoName: "Create",
		Method: "POST",
		Path:   "/pets",
		BodyParams: []parser.ParamData{
			{Name: "name", GoVarName: "Name", FlagName: "name", GoType: "string", FlagFunc: "StringVar", DefaultLiteral: `""`, DefaultCmp: `!= ""`},
			{Name: "age", GoVarName: "Age", FlagName: "age", GoType: "int", FlagFunc: "IntVar", DefaultLiteral: "0", DefaultCmp: "!= 0"},
		},
	})
	return d
}

// TestGenerateREST_BodyParams_BytesImport verifies that when HasBodyParams is true
// the generated code imports "bytes".
func TestGenerateREST_BodyParams_BytesImport(t *testing.T) {
	out, err := generateREST(bodyRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, `"bytes"`) {
		t.Error(`generated code must import "bytes" when HasBodyParams is true`)
	}
}

// TestGenerateREST_BodyParams_NoBytes_WhenAbsent verifies that without body params
// the "bytes" package is NOT imported (would cause compile error).
func TestGenerateREST_BodyParams_NoBytes_WhenAbsent(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	if strings.Contains(string(out), `"bytes"`) {
		t.Error(`generated code must NOT import "bytes" when there are no body params`)
	}
}

// TestGenerateREST_BodyParams_BuildsMap verifies that the generated op function
// builds a map[string]interface{} and marshals it for the request body.
func TestGenerateREST_BodyParams_BuildsMap(t *testing.T) {
	out, err := generateREST(bodyRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "reqBody := map[string]interface{}{}") {
		t.Error("generated code must declare 'reqBody := map[string]interface{}{}'")
	}
	if !strings.Contains(src, "json.Marshal(reqBody)") {
		t.Error("generated code must call json.Marshal(reqBody)")
	}
	if !strings.Contains(src, "bytes.NewReader(bodyBytes),") {
		t.Error("generated code must pass bytes.NewReader(bodyBytes) to doRequest")
	}
}

// TestGenerateREST_BodyParams_FlagsDeclared verifies that each body param gets
// a flag declaration in the generated op function.
func TestGenerateREST_BodyParams_FlagsDeclared(t *testing.T) {
	out, err := generateREST(bodyRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	src := string(out)
	for _, flag := range []string{`"name"`, `"age"`} {
		if !strings.Contains(src, flag) {
			t.Errorf("generated code must declare flag %s for body param", flag)
		}
	}
}

// TestGenerateREST_NoBodyParams_NilBody verifies that ops without body params
// still call doRequest with nil for both body and headers.
func TestGenerateREST_NoBodyParams_NilBody(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "doRequest(\"GET\", u.String(), nil, nil)") {
		t.Error("ops without body or header params must call doRequest(..., nil, nil)")
	}
}

// headerRESTData returns a CLIData with a GET operation that has one header param.
func headerRESTData() *parser.CLIData {
	d := minimalRESTData()
	d.Commands[0].Operations[0].HeaderParams = []parser.ParamData{
		{Name: "X-Tenant-ID", GoVarName: "XTenantID", FlagName: "x-tenant-id", GoType: "string", FlagFunc: "StringVar", DefaultLiteral: `""`, DefaultCmp: `!= ""`},
	}
	return d
}

// TestGenerateREST_HeaderParams_FlagDeclared verifies that a header param gets
// a StringVar flag declaration in the generated op function.
func TestGenerateREST_HeaderParams_FlagDeclared(t *testing.T) {
	out, err := generateREST(headerRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, `"x-tenant-id"`) {
		t.Error(`generated code must declare flag "x-tenant-id" for header param X-Tenant-ID`)
	}
}

// TestGenerateREST_HeaderParams_BuildsMap verifies that when header params are
// present the generated op function builds a hdrs map and passes it to doRequest.
func TestGenerateREST_HeaderParams_BuildsMap(t *testing.T) {
	out, err := generateREST(headerRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, `hdrs := map[string]string{}`) {
		t.Error(`generated code must declare 'hdrs := map[string]string{}'`)
	}
	if !strings.Contains(src, `hdrs["X-Tenant-ID"]`) {
		t.Errorf("generated code must set hdrs[%q]", "X-Tenant-ID")
	}
	if !strings.Contains(src, "doRequest(\"GET\", u.String(), nil, hdrs)") {
		t.Error("generated code must pass hdrs as headers arg to doRequest")
	}
}

// TestGenerateREST_HeaderParams_DoRequestAcceptsMap verifies that doRequest
// has a headers map[string]string parameter and iterates over it.
func TestGenerateREST_HeaderParams_DoRequestAcceptsMap(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	src := string(out)
	if !strings.Contains(src, "func doRequest(method, rawURL string, body io.Reader, headers map[string]string)") {
		t.Error("doRequest must accept a 'headers map[string]string' parameter")
	}
	if !strings.Contains(src, "for k, v := range headers") {
		t.Error("doRequest must iterate over headers with 'for k, v := range headers'")
	}
	if !strings.Contains(src, "req.Header.Set(k, v)") {
		t.Error("doRequest must call req.Header.Set(k, v) for each custom header")
	}
}

// TestGenerateREST_HTTPClientTimeout verifies that the generated code declares
// a package-level http.Client with a non-zero Timeout so requests cannot hang
// indefinitely.
func TestGenerateREST_HTTPClientTimeout(t *testing.T) {
	out, err := generateREST(minimalRESTData())
	if err != nil {
		t.Fatalf("generateREST returned error: %v", err)
	}
	src := string(out)

	if strings.Contains(src, "http.DefaultClient") {
		t.Error("generated code must not use http.DefaultClient (no timeout)")
	}
	if !strings.Contains(src, "http.Client{Timeout:") {
		t.Error("generated code must declare an http.Client with a Timeout field")
	}
	if !strings.Contains(src, `"time"`) {
		t.Error(`generated code must import "time" for the timeout duration`)
	}
	if !strings.Contains(src, "time.Second") {
		t.Error("generated code must use time.Second in the Timeout value")
	}
}
