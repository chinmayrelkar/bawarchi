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
		Name:       "test-api",
		SpecSource: "https://example.com/openapi.yaml",
		Transport:  "rest",
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
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

// TestCaseInsensitiveLookup verifies that Get, Add (duplicate check), Update,
// and Remove all treat names as case-insensitive so "MyAPI" and "myapi" refer
// to the same entry.
func TestCaseInsensitiveLookup(t *testing.T) {
	restore := withTempHome(t)
	defer restore()

	entry := Entry{
		Name:       "MyAPI",
		SpecSource: "https://example.com/openapi.yaml",
		Transport:  "rest",
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := Add(entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Get with different casing should succeed.
	for _, name := range []string{"myapi", "MYAPI", "MyApi", "MyAPI"} {
		got, err := Get(name)
		if err != nil {
			t.Errorf("Get(%q) unexpected error: %v", name, err)
			continue
		}
		if got.Name != "MyAPI" {
			t.Errorf("Get(%q) Name = %q, want %q", name, got.Name, "MyAPI")
		}
	}

	// Add with a case variant should return "already exists".
	dupErr := Add(Entry{Name: "myapi", SpecSource: "https://other.com", Transport: "rest"})
	if dupErr == nil {
		t.Error("Add with case-variant name should return 'already exists' error")
	}

	// Update via case variant should work.
	if err := Update("myapi", "https://new.example.com", ""); err != nil {
		t.Errorf("Update with case-variant name: %v", err)
	}

	// Remove via case variant should work.
	if err := Remove("MYAPI"); err != nil {
		t.Errorf("Remove with case-variant name: %v", err)
	}
	entries, _ := Load()
	if len(entries) != 0 {
		t.Errorf("after Remove, expected 0 entries, got %d", len(entries))
	}
}

// TestSaveAtomicNoTempFiles verifies that no .registry-*.tmp scratch files are
// left in the registry directory after a successful Save call.
func TestSaveAtomicNoTempFiles(t *testing.T) {
	restore := withTempHome(t)
	defer restore()

	entry := Entry{
		Name:       "mycli",
		SpecSource: "https://example.com/openapi.yaml",
		Transport:  "rest",
		AddedAt:    time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := Save([]Entry{entry}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	pattern := filepath.Join(Dir(), ".registry-*.tmp")
	leftover, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(leftover) > 0 {
		t.Errorf("temp files left behind after Save: %v", leftover)
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
