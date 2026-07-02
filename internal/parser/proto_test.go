package parser

import (
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

// TestParseProto_NoAuth_MidFile verifies that // @noauth placed mid-file
// (not just on line 1) still causes CLIData.AuthEnvVar to be empty string.
// This guards against a regex that only matches at the start of file.
func TestParseProto_NoAuth_MidFile(t *testing.T) {
	proto := makeProto(`syntax = "proto3";
package com.example.v1;
// some comment
`, `// @noauth
`)
	cli, err := ParseProto(proto, "test.proto")
	if err != nil {
		t.Fatalf("ParseProto failed: %v", err)
	}
	if cli.AuthEnvVar != "" {
		t.Errorf("AuthEnvVar = %q, want empty string when @noauth is mid-file", cli.AuthEnvVar)
	}
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
