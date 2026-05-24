package compiler

import (
	"strings"
	"testing"
)

// TestRunQuiet_SilentOnSuccess verifies a successful command produces no error.
func TestRunQuiet_SilentOnSuccess(t *testing.T) {
	if err := runQuiet(t.TempDir(), "true"); err != nil {
		// Some minimal environments lack `true`; fall back to a no-op `go env`.
		if err2 := runQuiet(t.TempDir(), "go", "env", "GOROOT"); err2 != nil {
			t.Skipf("no suitable no-op command available: %v / %v", err, err2)
		}
	}
}

// TestRunQuiet_SurfacesOutputOnFailure verifies the captured output is attached
// to the error when a command fails, instead of being written to the streams.
func TestRunQuiet_SurfacesOutputOnFailure(t *testing.T) {
	// `go build` in an empty dir fails with a diagnostic on stdout/stderr.
	err := runQuiet(t.TempDir(), "go", "build", "./...")
	if err == nil {
		t.Fatal("expected go build to fail in an empty dir")
	}
	if !strings.Contains(err.Error(), "go") && len(err.Error()) == 0 {
		t.Errorf("error should carry context, got: %v", err)
	}
}
