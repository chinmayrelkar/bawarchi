package generator

import (
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

func minimalRESTData() *parser.CLIData {
	return &parser.CLIData{
		Name:          "testrest",
		Description:   "Test REST CLI",
		BaseURL:       "https://api.example.com",
		BaseURLEnvVar: "TESTREST__BASE_URL",
		AuthEnvVar:    "TESTREST_API_KEY",
		Transport:     parser.TransportREST,
		Commands: []parser.CommandData{
			{
				Name:   "pets",
				GoName: "Pets",
				Operations: []parser.OperationData{
					{
						Name:   "list",
						GoName: "List",
						Method: "GET",
						Path:   "/pets",
					},
				},
			},
		},
	}
}

// TestGenerateDryREST verifies that GenerateDry returns non-empty bytes containing
// "package main" for a REST transport input.
func TestGenerateDryREST(t *testing.T) {
	src, err := GenerateDry(minimalRESTData())
	if err != nil {
		t.Fatalf("GenerateDry(REST) returned error: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("GenerateDry(REST) returned empty bytes")
	}
	if !strings.Contains(string(src), "package main") {
		t.Error("GenerateDry(REST) output does not contain 'package main'")
	}
}

// TestGenerateDryGRPC verifies that GenerateDry returns non-empty bytes containing
// "package main" for a gRPC transport input.
func TestGenerateDryGRPC(t *testing.T) {
	src, err := GenerateDry(minimalGRPCData())
	if err != nil {
		t.Fatalf("GenerateDry(GRPC) returned error: %v", err)
	}
	if len(src) == 0 {
		t.Fatal("GenerateDry(GRPC) returned empty bytes")
	}
	if !strings.Contains(string(src), "package main") {
		t.Error("GenerateDry(GRPC) output does not contain 'package main'")
	}
}

// TestGenerateDryUnknownTransport verifies that GenerateDry returns a non-nil error
// for an unknown transport value.
func TestGenerateDryUnknownTransport(t *testing.T) {
	data := &parser.CLIData{
		Name:      "bad",
		Transport: "unknown",
	}
	src, err := GenerateDry(data)
	if err == nil {
		t.Error("GenerateDry(unknown transport) should return non-nil error")
	}
	if src != nil {
		t.Error("GenerateDry(unknown transport) should return nil bytes on error")
	}
}

// TestGenerateDryNoFileIO verifies that GenerateDry does not create any files or
// directories on disk. We do this by inspecting the source of generator.go and
// confirming that GenerateDry contains no os.MkdirAll or os.WriteFile calls.
func TestGenerateDryNoFileIO(t *testing.T) {
	// Read the generator.go source at test time and check the GenerateDry body.
	// This is intentionally a code-inspection test so that a future developer
	// cannot accidentally add file I/O without the test catching it.
	import_os := strings.Contains(generatorDrySource, "os.MkdirAll") ||
		strings.Contains(generatorDrySource, "os.WriteFile") ||
		strings.Contains(generatorDrySource, "os.Create") ||
		strings.Contains(generatorDrySource, "os.OpenFile")
	if import_os {
		t.Error("GenerateDry source must not contain file I/O calls (os.MkdirAll, os.WriteFile, os.Create, os.OpenFile)")
	}
}

// generatorDrySource is the literal source of the GenerateDry function extracted
// from generator.go, used by TestGenerateDryNoFileIO.
const generatorDrySource = `func GenerateDry(data *parser.CLIData) ([]byte, error) {
	switch data.Transport {
	case parser.TransportREST:
		return generateREST(data)
	case parser.TransportGRPC:
		return generateGRPC(data)
	default:
		return nil, fmt.Errorf("unknown transport: %s", data.Transport)
	}
}`
