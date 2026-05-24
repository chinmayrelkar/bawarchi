package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad_HTTPRejected_AC confirms that Load("http://...") returns a non-nil
// error containing "https://" and does NOT contain "dial" (no network call).
func TestLoad_HTTPRejected_AC(t *testing.T) {
	_, err := Load("http://example.com/spec.yaml")
	if err == nil {
		t.Fatal("expected non-nil error for http:// URL, got nil")
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Fatalf("error must mention 'https://', got: %q", err.Error())
	}
	// Confirm no network dial occurred — rejection is purely local.
	if strings.Contains(err.Error(), "dial") {
		t.Fatalf("error suggests a network call was made for http:// input; error: %q", err.Error())
	}
}

// TestParseSource_HTTPRejected confirms that the http:// rejection propagates
// through the public entry point ParseSource.
func TestParseSource_HTTPRejected(t *testing.T) {
	_, err := ParseSource("http://evil.com/spec.yaml")
	if err == nil {
		t.Fatal("expected non-nil error from ParseSource for http:// URL, got nil")
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Fatalf("error from ParseSource must mention 'https://', got: %q", err.Error())
	}
}

// TestLoad_FileAccepted verifies that the file-path branch is not broken by
// the http:// guard. It writes a minimal YAML file and loads it.
func TestLoad_FileAccepted(t *testing.T) {
	dir := t.TempDir()
	tmpFile := filepath.Join(dir, "spec.yaml")

	minimalYAML := []byte("openapi: \"3.0.0\"\ninfo:\n  title: Test\n  version: \"1.0\"\npaths: {}\n")
	if err := os.WriteFile(tmpFile, minimalYAML, 0600); err != nil {
		t.Fatalf("failed to create temp spec file: %v", err)
	}

	data, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load() returned error for a valid file path: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Load() returned empty data for a valid file path")
	}
}

// TestNoNolintGosecInParser ensures the //nolint:gosec comment has been
// removed from parser.go as required by the fix.
func TestNoNolintGosecInParser(t *testing.T) {
	src, err := os.ReadFile("parser.go")
	if err != nil {
		t.Fatalf("could not read parser.go: %v", err)
	}
	content := string(src)
	if strings.Contains(content, "nolint") {
		t.Error("parser.go still contains 'nolint' — it must be removed")
	}
	if strings.Contains(content, "gosec") {
		t.Error("parser.go still contains 'gosec' — it must be removed")
	}
}
