package parser

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestParseProto_NoAuth_AuthEnvVarEmpty verifies that // @noauth causes
// CLIData.AuthEnvVar to be empty string.
func TestParseProto_NoAuth_AuthEnvVarEmpty(t *testing.T) {
	proto := makeProto(`// @noauth
`, "")
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	if cli.AuthEnvVar != "" {
		t.Errorf("AuthEnvVar = %q, want empty string when @noauth is set", cli.AuthEnvVar)
	}
}

// TestParseProto_WithoutNoAuth_AuthEnvVarNonEmpty verifies that without @noauth
// CLIData.AuthEnvVar is non-empty and ends with __TOKEN.
func TestParseProto_WithoutNoAuth_AuthEnvVarNonEmpty(t *testing.T) {
	proto := makeProto(`syntax = "proto3";
`, "")
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	if cli.AuthEnvVar == "" {
		t.Error("AuthEnvVar must be non-empty when @noauth is not set")
	}
	if !strings.HasSuffix(cli.AuthEnvVar, "__TOKEN") {
		t.Errorf("AuthEnvVar = %q, want suffix __TOKEN", cli.AuthEnvVar)
	}
}

// minimal proto helper
func makeProto(header, serviceBlock string) []byte {
	return []byte(header + `
service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply);
}
message HelloRequest { string name = 1; }
message HelloReply { string message = 1; }
` + serviceBlock)
}

// TestParseProto_ServiceAnnotation verifies that // @service: com.example.v1
// causes GRPCService to be 'com.example.v1.Greeter'.
func TestParseProto_ServiceAnnotation(t *testing.T) {
	proto := makeProto(`// @service: com.example.v1
`, "")
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	op := cli.Commands[0].Operations[0]
	want := "com.example.v1.Greeter"
	if op.GRPCService != want {
		t.Errorf("GRPCService = %q, want %q", op.GRPCService, want)
	}
}

// TestParseProto_PackageFallback verifies that without @service but with
// 'package com.example.v1;' the GRPCService is 'com.example.v1.Greeter'.
func TestParseProto_PackageFallback(t *testing.T) {
	proto := makeProto(`syntax = "proto3";
package com.example.v1;
`, "")
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	op := cli.Commands[0].Operations[0]
	want := "com.example.v1.Greeter"
	if op.GRPCService != want {
		t.Errorf("GRPCService = %q, want %q", op.GRPCService, want)
	}
}

// TestParseProto_NoLeadingDot verifies that with neither @service nor package
// the GRPCService is the bare service name with no leading dot.
func TestParseProto_NoLeadingDot(t *testing.T) {
	proto := []byte(`syntax = "proto3";
service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply);
}
message HelloRequest { string name = 1; }
message HelloReply { string message = 1; }
`)
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	op := cli.Commands[0].Operations[0]
	if strings.HasPrefix(op.GRPCService, ".") {
		t.Errorf("GRPCService %q must not start with a leading dot", op.GRPCService)
	}
	if op.GRPCService != "Greeter" {
		t.Errorf("GRPCService = %q, want %q", op.GRPCService, "Greeter")
	}
}

// TestParseProto_ServiceAnnotationOverridesPackage verifies that @service takes
// precedence over the package declaration when both are present.
func TestParseProto_ServiceAnnotationOverridesPackage(t *testing.T) {
	proto := makeProto(`syntax = "proto3";
// @service: override.path
package com.example.v1;
`, "")
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	op := cli.Commands[0].Operations[0]
	want := "override.path.Greeter"
	if op.GRPCService != want {
		t.Errorf("GRPCService = %q, want %q", op.GRPCService, want)
	}
}

// TestExtractProtoOption_ServiceAnnotation verifies that extractProtoOption
// correctly extracts the servicePath from a @service comment.
func TestExtractProtoOption_ServiceAnnotation(t *testing.T) {
	content := `// @service: com.acme.v2
// @server: api.example.com:443
`
	servicePath, serverAddr := extractProtoOption(content)
	if servicePath != "com.acme.v2" {
		t.Errorf("servicePath = %q, want %q", servicePath, "com.acme.v2")
	}
	if serverAddr != "api.example.com:443" {
		t.Errorf("serverAddr = %q, want %q", serverAddr, "api.example.com:443")
	}
}

// TestExtractProtoOption_NoAnnotations verifies that with no annotations the
// servicePath falls back to the package declaration (full dot-separated string).
func TestExtractProtoOption_NoAnnotations(t *testing.T) {
	content := `syntax = "proto3";
package foo.bar.v1;
`
	servicePath, serverAddr := extractProtoOption(content)
	if servicePath != "foo.bar.v1" {
		t.Errorf("servicePath = %q, want %q", servicePath, "foo.bar.v1")
	}
	// Default server address when no @server annotation present
	if serverAddr != "localhost:50051" {
		t.Errorf("serverAddr = %q, want %q", serverAddr, "localhost:50051")
	}
}

// minimalProto returns a minimal .proto file with the given package name and an optional
// @server annotation.
func minimalProto(pkgName, serverAnnotation, serviceName string) []byte {
	ann := ""
	if serverAnnotation != "" {
		ann = "// @server: " + serverAnnotation + "\n"
	}
	return []byte(`syntax = "proto3";
package ` + pkgName + `;
` + ann + `
service ` + serviceName + ` {
  rpc SayHello (HelloRequest) returns (HelloReply);
}

message HelloRequest {
  string name = 1;
}

message HelloReply {
  string message = 1;
}
`)
}

// TestParseProto_BaseURLEnvVarSet verifies that ParseProto populates BaseURLEnvVar
// from the package name with the expected naming convention.
func TestParseProto_BaseURLEnvVarSet(t *testing.T) {
	// Suppress stderr warnings during test
	old := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		w.Close()
		os.Stderr = old
	}()

	proto := minimalProto("testpkg", "myhost:50051", "TestService")
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}

	want := "TESTPKG__SERVER_ADDR"
	if cli.BaseURLEnvVar != want {
		t.Errorf("BaseURLEnvVar = %q, want %q", cli.BaseURLEnvVar, want)
	}
}

// TestParseProto_ServerAnnotation_NoWarning verifies that when a @server: annotation is
// present, no warning is written to stderr.
func TestParseProto_ServerAnnotation_NoWarning(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w

	proto := minimalProto("mypkg", "realhost:9090", "MyService")
	_, parseErr := ParseProto(proto, "my.proto")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	if parseErr != nil {
		t.Fatalf("ParseProto failed: %v", parseErr)
	}
	if buf.Len() > 0 {
		t.Errorf("expected no stderr output when @server annotation is present, got: %q", buf.String())
	}
}
