package main

import (
	"os"
	"strings"
	"testing"
)

// TestCookAndRegisterBinDirMode verifies that cookAndRegister creates binDir with 0700.
func TestCookAndRegisterBinDirMode(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	if !strings.Contains(string(src), "os.MkdirAll(binDir, 0700)") {
		t.Error("main.go cookAndRegister must contain os.MkdirAll(binDir, 0700)")
	}
}

// TestInstallDirPermissionKept verifies that installCmd keeps 0755 for PATH directories.
func TestInstallDirPermissionKept(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	if !strings.Contains(string(src), "os.MkdirAll(installDir, 0755)") {
		t.Error("main.go installCmd must contain os.MkdirAll(installDir, 0755)")
	}
}
