package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheSpec_RoundTrip(t *testing.T) {
	defer withTempHome(t)()

	want := []byte("openapi: 3.0.0\ninfo:\n  title: X\n")
	if err := CacheSpec("myapi", want); err != nil {
		t.Fatalf("CacheSpec: %v", err)
	}

	got, err := CachedSpec("myapi")
	if err != nil {
		t.Fatalf("CachedSpec: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("cached spec = %q, want %q", got, want)
	}
}

func TestCacheSpec_FileIsOwnerOnly(t *testing.T) {
	defer withTempHome(t)()
	if err := CacheSpec("perm", []byte("x")); err != nil {
		t.Fatalf("CacheSpec: %v", err)
	}
	info, err := os.Stat(filepath.Join(SpecDir(), "perm.spec"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("cached spec perm = %o, want 0600", perm)
	}
}

func TestRemoveCachedSpec(t *testing.T) {
	defer withTempHome(t)()
	if err := CacheSpec("gone", []byte("x")); err != nil {
		t.Fatalf("CacheSpec: %v", err)
	}
	if err := RemoveCachedSpec("gone"); err != nil {
		t.Fatalf("RemoveCachedSpec: %v", err)
	}
	if _, err := CachedSpec("gone"); err == nil {
		t.Error("expected error reading removed cached spec")
	}
	// Removing a non-existent cache must be a no-op (nil error).
	if err := RemoveCachedSpec("never-existed"); err != nil {
		t.Errorf("RemoveCachedSpec on missing file should be nil, got %v", err)
	}
}
