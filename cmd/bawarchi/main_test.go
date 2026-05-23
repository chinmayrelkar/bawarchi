package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/registry"
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

// minimalOpenAPISpec is a tiny but valid OpenAPI 3 document with one GET
// endpoint so the generator produces compilable Go code (all imports are used).
const minimalOpenAPISpec = `openapi: "3.0.0"
info:
  title: testapi
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /items:
    get:
      tags: [items]
      operationId: listItems
      summary: List items
`

// setupRegistryWithSpec seeds the registry with one entry pointing at a local
// spec file and returns the path to the registry JSON file so callers can
// manipulate its permissions to induce failures in registry.Update.
func setupRegistryWithSpec(t *testing.T, name string) (specFile, regJSONFile string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	specFile = filepath.Join(tmpDir, "spec.yaml")
	if err := os.WriteFile(specFile, []byte(minimalOpenAPISpec), 0644); err != nil {
		t.Fatalf("writing spec file: %v", err)
	}

	if err := registry.Add(registry.Entry{
		Name:       name,
		SpecSource: specFile,
		Transport:  "rest",
	}); err != nil {
		t.Fatalf("seeding registry: %v", err)
	}

	regJSONFile = filepath.Join(registry.Dir(), "registry.json")
	return specFile, regJSONFile
}

// makeRegistryDirReadOnly pre-creates the subdirectories that cookAndRegister
// needs (so it can succeed), then makes the registry directory itself
// non-writable so that os.CreateTemp inside registry.Save fails.  It registers
// a cleanup that restores write permission before the test exits.
//
// Background: registry.Save now uses an atomic write-then-rename rather than
// os.WriteFile.  os.Rename replaces the destination regardless of the
// destination file's permission bits — only the directory write bit matters.
// Making the directory non-writable is therefore the correct way to induce a
// Save failure without touching the file itself.
func makeRegistryDirReadOnly(t *testing.T, name string) {
	t.Helper()
	regDir := registry.Dir() // capture before HOME is restored by t.Setenv cleanup

	// Pre-create every directory that cookAndRegister touches so that step
	// succeeds even with a non-writable registry parent directory.
	for _, d := range []string{
		registry.BinDir(),
		filepath.Join(registry.SrcDir(), name),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("pre-create %s: %v", d, err)
		}
	}

	if err := os.Chmod(regDir, 0500); err != nil {
		t.Fatalf("chmod %s: %v", regDir, err)
	}
	t.Cleanup(func() { os.Chmod(regDir, 0700) }) //nolint:errcheck
}

// TestUpdateCmdRegistryUpdateErrorSurfaced verifies that when registry.Update
// returns an error the updateCmd RunE propagates it wrapped as "updating registry: …"
// instead of silently ignoring it.
func TestUpdateCmdRegistryUpdateErrorSurfaced(t *testing.T) {
	setupRegistryWithSpec(t, "testcli")
	makeRegistryDirReadOnly(t, "testcli")

	cmd := updateCmd()
	cmd.SetArgs([]string{"testcli"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error from updateCmd when registry.Update fails, got nil")
	}
	if !strings.Contains(err.Error(), "updating registry") {
		t.Fatalf("expected error to contain %q, got: %v", "updating registry", err)
	}
}

// TestUpdateCmdRegistryUpdateErrorIsWrapped verifies the error uses %w so it
// remains unwrappable.
func TestUpdateCmdRegistryUpdateErrorIsWrapped(t *testing.T) {
	setupRegistryWithSpec(t, "testcli2")
	makeRegistryDirReadOnly(t, "testcli2")

	cmd := updateCmd()
	cmd.SetArgs([]string{"testcli2"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	if errors.Unwrap(err) == nil {
		t.Fatalf("expected a wrapped error (%%w), but errors.Unwrap returned nil for: %v", err)
	}
}

// TestUpdateCmdNoNolintErrcheck verifies that the //nolint:errcheck suppression
// has been removed from the registry.Update call site in main.go.
func TestUpdateCmdNoNolintErrcheck(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	src := string(data)

	for _, line := range strings.Split(src, "\n") {
		if strings.Contains(line, "registry.Update") && strings.Contains(line, "nolint") {
			t.Fatalf("found //nolint suppression on registry.Update line: %q", strings.TrimSpace(line))
		}
	}
}
