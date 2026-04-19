package compiler

import (
	"os"
	"strings"
	"testing"
)

// TestCompileSrcDirMode verifies that the srcDir is created with 0700 (not 0755).
func TestCompileSrcDirMode(t *testing.T) {
	src, err := os.ReadFile("compiler.go")
	if err != nil {
		t.Fatalf("reading compiler.go: %v", err)
	}
	if !strings.Contains(string(src), "os.MkdirAll(srcDir, 0700)") {
		t.Error("compiler.go must contain os.MkdirAll(srcDir, 0700)")
	}
}
