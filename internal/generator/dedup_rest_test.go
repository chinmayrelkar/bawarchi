package generator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

// TestGenerateREST_NoDuplicateSwitchCases detects duplicate case strings in the
// generated REST switch statement by scanning the generated source for
// repeated `case "<name>":` patterns within a single command function.
func TestGenerateREST_NoDuplicateSwitchCases(t *testing.T) {
	// Build CLIData with a single command that has two operations sharing the
	// same HTTP method but different paths and names (as dedup should produce).
	data := &parser.CLIData{
		Name:          "testapi",
		Description:   "Test API",
		BaseURL:       "https://api.example.com",
		BaseURLEnvVar: "TESTAPI__BASE_URL",
		AuthEnvVar:    "TESTAPI__API_KEY",
		AuthSetup:     `req.Header.Set("Authorization", "Bearer "+key)`,
		Transport:     parser.TransportREST,
		Commands: []parser.CommandData{
			{
				Name:   "users",
				GoName: "Users",
				Operations: []parser.OperationData{
					{
						Name:   "get-users",
						GoName: "GetUsers",
						Method: "GET",
						Path:   "/users",
					},
					{
						Name:   "get-users-id",
						GoName: "GetUsersId",
						Method: "GET",
						Path:   "/users/{id}",
					},
				},
			},
		},
	}

	src, err := generateREST(data)
	if err != nil {
		t.Fatalf("generateREST failed: %v", err)
	}
	code := string(src)

	// Scan for duplicate `case "<name>":` labels.
	seenCases := map[string]int{}
	for _, line := range strings.Split(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "case \"") && strings.HasSuffix(trimmed, "\":") {
			label := trimmed[len(`case "`):strings.LastIndex(trimmed, `":`)]
			seenCases[label]++
		}
	}
	for label, count := range seenCases {
		if count > 1 {
			t.Errorf("duplicate case %q appears %d times in generated source", label, count)
		}
	}
}

// TestGenerateREST_DedupedNamesAreDistinct verifies that when two operations
// in the same command have names that would collide, the generated source
// contains both (distinct) case labels.
func TestGenerateREST_DedupedNamesAreDistinct(t *testing.T) {
	// Simulate what the parser produces after dedup: "get-2" as the disambiguated name.
	data := &parser.CLIData{
		Name:          "testapi",
		Description:   "Test API",
		BaseURL:       "https://api.example.com",
		BaseURLEnvVar: "TESTAPI__BASE_URL",
		AuthEnvVar:    "TESTAPI__API_KEY",
		AuthSetup:     `req.Header.Set("Authorization", "Bearer "+key)`,
		Transport:     parser.TransportREST,
		Commands: []parser.CommandData{
			{
				Name:   "items",
				GoName: "Items",
				Operations: []parser.OperationData{
					{
						Name:   "get-items",
						GoName: "GetItems",
						Method: "GET",
						Path:   "/items",
					},
					{
						Name:   "get-items-id",
						GoName: "GetItemsId",
						Method: "GET",
						Path:   "/items/{id}",
					},
				},
			},
		},
	}

	src, err := generateREST(data)
	if err != nil {
		t.Fatalf("generateREST failed: %v", err)
	}
	code := string(src)

	for _, name := range []string{"get-items", "get-items-id"} {
		expected := fmt.Sprintf(`case "%s":`, name)
		if !strings.Contains(code, expected) {
			t.Errorf("generated source missing expected case label %q", expected)
		}
	}
}
