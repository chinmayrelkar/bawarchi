package parser

import (
	"os"
	"strings"
	"testing"
)

// TestAuthFromSchemes_BearerBeatsApiKey verifies that http/bearer wins over
// apiKey/header and that the returned envVar ends in __TOKEN and setup
// contains "Bearer "+key.
func TestAuthFromSchemes_BearerBeatsApiKey(t *testing.T) {
	schemes := map[string]oaSecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer"},
		"apiKeyAuth": {Type: "apiKey", In: "header", Name: "X-API-Key"},
	}
	var ev, setup string
	stderr := captureStderr(func() {
		ev, setup = authFromSchemes(schemes, "My API")
	})
	if !strings.HasSuffix(ev, "__TOKEN") {
		t.Errorf("expected envVar to end with __TOKEN, got %q", ev)
	}
	if !strings.Contains(setup, `"Bearer "`) {
		t.Errorf("expected setup to contain 'Bearer ', got %q", setup)
	}
	if !strings.Contains(stderr, "bearerAuth") {
		t.Errorf("expected stderr to contain scheme name 'bearerAuth', got %q", stderr)
	}
}

// TestAuthFromSchemes_BasicBeatsApiKey verifies that http/basic wins over
// apiKey/header and that envVar ends in __CREDENTIALS and setup contains base64.
func TestAuthFromSchemes_BasicBeatsApiKey(t *testing.T) {
	schemes := map[string]oaSecurityScheme{
		"basicAuth":  {Type: "http", Scheme: "basic"},
		"apiKeyAuth": {Type: "apiKey", In: "header", Name: "X-API-Key"},
	}
	var ev, setup string
	stderr := captureStderr(func() {
		ev, setup = authFromSchemes(schemes, "My API")
	})
	if !strings.HasSuffix(ev, "__CREDENTIALS") {
		t.Errorf("expected envVar to end with __CREDENTIALS, got %q", ev)
	}
	if !strings.Contains(setup, "base64") {
		t.Errorf("expected setup to contain 'base64', got %q", setup)
	}
	if !strings.Contains(stderr, "basicAuth") {
		t.Errorf("expected stderr to contain scheme name 'basicAuth', got %q", stderr)
	}
}

// TestAuthFromSchemes_ApiKeyHeaderBeatsQuery verifies that apiKey/header wins
// over apiKey/query.
func TestAuthFromSchemes_ApiKeyHeaderBeatsQuery(t *testing.T) {
	schemes := map[string]oaSecurityScheme{
		"headerKey": {Type: "apiKey", In: "header", Name: "X-API-Key"},
		"queryKey":  {Type: "apiKey", In: "query", Name: "api_key"},
	}
	var _, setup string
	stderr := captureStderr(func() {
		_, setup = authFromSchemes(schemes, "My API")
	})
	if !strings.Contains(setup, "X-API-Key") {
		t.Errorf("expected setup to use header key X-API-Key, got %q", setup)
	}
	if !strings.Contains(stderr, "headerKey") {
		t.Errorf("expected stderr to contain 'headerKey', got %q", stderr)
	}
}

// TestAuthFromSchemes_Determinism calls authFromSchemes 100 times on the same
// multi-scheme map and checks that the result is always identical.
func TestAuthFromSchemes_Determinism(t *testing.T) {
	schemes := map[string]oaSecurityScheme{
		"bearerAuth": {Type: "http", Scheme: "bearer"},
		"apiKeyAuth": {Type: "apiKey", In: "header", Name: "X-API-Key"},
	}
	var firstEV, firstSetup string
	captureStderr(func() {
		firstEV, firstSetup = authFromSchemes(schemes, "My API")
	})
	for i := 0; i < 100; i++ {
		var ev, setup string
		captureStderr(func() {
			ev, setup = authFromSchemes(schemes, "My API")
		})
		if ev != firstEV || setup != firstSetup {
			t.Fatalf("iteration %d: got (%q, %q), want (%q, %q)", i+1, ev, setup, firstEV, firstSetup)
		}
	}
}

// TestAuthFromSchemes_EmptySchemes verifies that an empty map silently returns
// the default Bearer fallback and writes nothing to stderr.
func TestAuthFromSchemes_EmptySchemes(t *testing.T) {
	var ev, setup string
	stderr := captureStderr(func() {
		ev, setup = authFromSchemes(map[string]oaSecurityScheme{}, "My API")
	})
	if !strings.Contains(setup, `"Bearer "`) {
		t.Errorf("expected default Bearer setup, got %q", setup)
	}
	_ = ev
	if stderr != "" {
		t.Errorf("expected no stderr for empty schemes, got %q", stderr)
	}
}

// TestAuthFromSchemes_SortStringsPresent is a source-scan test that verifies
// openapi.go uses sort.Strings (the determinism mechanism).
func TestAuthFromSchemes_SortStringsPresent(t *testing.T) {
	src, err := os.ReadFile("openapi.go")
	if err != nil {
		t.Fatalf("could not read openapi.go: %v", err)
	}
	if !strings.Contains(string(src), "sort.Strings") {
		t.Error("openapi.go does not contain 'sort.Strings' — deterministic iteration is missing")
	}
}
