package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// overrideHome temporarily redirects the home directory used by the registry
// package to a temp dir so tests don't touch the real ~/.bawarchi directory.
func withTempHome(t *testing.T) func() {
	t.Helper()
	tmp := t.TempDir()
	orig, set := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", tmp); err != nil {
		t.Fatalf("setenv HOME: %v", err)
	}
	return func() {
		if set {
			os.Setenv("HOME", orig)
		} else {
			os.Unsetenv("HOME")
		}
	}
}

func TestSavePermissions(t *testing.T) {
	restore := withTempHome(t)
	defer restore()

	entry := Entry{
		Name:      "test-api",
		SpecSource: "https://example.com/openapi.yaml",
		Transport: "rest",
		AddedAt:   time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := Save([]Entry{entry}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Check registry.json is 0600 (owner read/write only).
	fi, err := os.Stat(registryFile())
	if err != nil {
		t.Fatalf("stat registry file: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("registry.json permissions = %04o, want 0600", got)
	}

	// Check the ~/.bawarchi directory is 0700 (owner only).
	di, err := os.Stat(Dir())
	if err != nil {
		t.Fatalf("stat registry dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0700 {
		t.Errorf(".bawarchi dir permissions = %04o, want 0700", got)
	}

	// Sanity: confirm the wrong old values are NOT present.
	if fi.Mode().Perm() == 0644 {
		t.Errorf("registry.json is still world-readable (0644)")
	}
	if di.Mode().Perm() == 0755 {
		t.Errorf(".bawarchi dir is still world-readable (0755)")
	}
}

// TestSavePermissionsDir verifies the directory is created fresh with the
// correct mode even when the directory did not previously exist.
func TestSavePermissionsDir(t *testing.T) {
	restore := withTempHome(t)
	defer restore()

	// Ensure the dir doesn't exist yet.
	if err := os.RemoveAll(Dir()); err != nil {
		t.Fatalf("remove dir: %v", err)
	}

	if err := Save(nil); err != nil {
		t.Fatalf("Save: %v", err)
	}

	di, err := os.Stat(filepath.Join(Dir()))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := di.Mode().Perm(); got != 0700 {
		t.Errorf(".bawarchi dir permissions = %04o, want 0700", got)
	}
}
