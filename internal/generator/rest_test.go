package generator

import (
	"strings"
	"testing"
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
