package parser

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// minimalSwagger2Spec returns a minimal Swagger 2.0 YAML spec with the given schemes.
func minimalSwagger2Spec(schemes []string) swagger2Spec {
	schemeList := ""
	for _, s := range schemes {
		schemeList += fmt.Sprintf("\n- %s", s)
	}
	_ = schemeList // not used — we build the struct directly
	return swagger2Spec{
		Host:     "api.example.com",
		BasePath: "/v1",
		Schemes:  schemes,
		Info:     oaInfo{Title: "Test API"},
	}
}

// captureStderr runs f() and returns whatever was written to os.Stderr during f.
func captureStderr(f func()) string {
	orig := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	f()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = orig
	return buf.String()
}

// TestSwagger2BaseURL_HttpsPresent_First verifies https is chosen when it is the first scheme.
func TestSwagger2BaseURL_HttpsPresent_First(t *testing.T) {
	spec := minimalSwagger2Spec([]string{"https", "http"})
	var result string
	stderr := captureStderr(func() {
		result = swagger2BaseURL(spec)
	})
	if !strings.HasPrefix(result, "https://") {
		t.Errorf("expected result to start with 'https://', got %q", result)
	}
	if stderr != "" {
		t.Errorf("expected no stderr warning when https is available, got %q", stderr)
	}
}

// TestSwagger2BaseURL_HttpsPresent_Last verifies https is chosen even when listed after http.
func TestSwagger2BaseURL_HttpsPresent_Last(t *testing.T) {
	spec := minimalSwagger2Spec([]string{"http", "https"})
	var result string
	stderr := captureStderr(func() {
		result = swagger2BaseURL(spec)
	})
	if !strings.HasPrefix(result, "https://") {
		t.Errorf("expected result to start with 'https://', got %q", result)
	}
	if stderr != "" {
		t.Errorf("expected no stderr warning when https is available, got %q", stderr)
	}
}

// TestSwagger2BaseURL_HttpOnly verifies http fallback with a stderr warning when https is absent.
func TestSwagger2BaseURL_HttpOnly(t *testing.T) {
	spec := minimalSwagger2Spec([]string{"http"})
	var result string
	stderr := captureStderr(func() {
		result = swagger2BaseURL(spec)
	})
	if !strings.HasPrefix(result, "http://") {
		t.Errorf("expected result to start with 'http://', got %q", result)
	}
	if strings.HasPrefix(result, "https://") {
		t.Errorf("result must NOT start with 'https://' for http-only spec, got %q", result)
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("expected a stderr warning when falling back to http, got %q", stderr)
	}
}

// TestSwagger2BaseURL_EmptySchemes verifies the default https when Schemes is empty.
func TestSwagger2BaseURL_EmptySchemes(t *testing.T) {
	spec := minimalSwagger2Spec([]string{})
	var result string
	stderr := captureStderr(func() {
		result = swagger2BaseURL(spec)
	})
	if !strings.HasPrefix(result, "https://") {
		t.Errorf("expected result to start with 'https://' when schemes empty, got %q", result)
	}
	if stderr != "" {
		t.Errorf("expected no stderr warning when schemes is empty, got %q", stderr)
	}
}

// TestSwagger2BaseURL_HttpsOnly verifies https with no warning when only https is listed.
func TestSwagger2BaseURL_HttpsOnly(t *testing.T) {
	spec := minimalSwagger2Spec([]string{"https"})
	var result string
	stderr := captureStderr(func() {
		result = swagger2BaseURL(spec)
	})
	if !strings.HasPrefix(result, "https://") {
		t.Errorf("expected result to start with 'https://', got %q", result)
	}
	if stderr != "" {
		t.Errorf("expected no stderr warning for https-only spec, got %q", stderr)
	}
}
