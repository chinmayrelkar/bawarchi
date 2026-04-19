package parser

import (
	"strings"
	"testing"
)

// TestLoad_HTTPRejected verifies that http:// URLs are hard-rejected before any
// network call is attempted. The returned error must mention https://.
func TestLoad_HTTPRejected(t *testing.T) {
	_, err := load("http://example.com/spec.yaml")
	if err == nil {
		t.Fatal("expected an error for http:// URL, got nil")
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Fatalf("error message should mention https://, got: %q", err.Error())
	}
	// Ensure the error is the rejection error and not a network error.
	if strings.Contains(err.Error(), "dial") || strings.Contains(err.Error(), "fetching spec") {
		t.Fatalf("a network call was made for http:// input; error: %q", err.Error())
	}
}

// TestLoad_HTTPSAllowed verifies that https:// URLs are not rejected by the guard.
// We don't actually dial; we just confirm the error (if any) is not our rejection error.
func TestLoad_HTTPSAllowed(t *testing.T) {
	_, err := load("https://example.com/spec.yaml")
	// A network error is fine here — what must NOT happen is the http:// rejection error.
	if err != nil && strings.Contains(err.Error(), "spec source must use https://") {
		t.Fatalf("https:// URL was incorrectly rejected: %v", err)
	}
}

// TestLoad_FileSource verifies that non-URL sources are treated as file paths
// (not rejected by the URL guard).
func TestLoad_FileSource(t *testing.T) {
	_, err := load("/nonexistent/path/spec.yaml")
	// Should get a file-not-found error, not the https rejection error.
	if err != nil && strings.Contains(err.Error(), "spec source must use https://") {
		t.Fatalf("file path was incorrectly rejected as http://: %v", err)
	}
}
