package registry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Entry struct {
	Name        string    `json:"name"`
	SpecSource  string    `json:"spec_source"`
	Transport   string    `json:"transport"`              // "rest" or "grpc"
	BaseURL     string    `json:"base_url,omitempty"`     // overridden base URL (e.g. EU endpoint)
	InstallPath string    `json:"install_path,omitempty"` // path of the symlink created by 'bawarchi install'
	AddedAt     time.Time `json:"added_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func Dir() string                      { return filepath.Join(home(), ".bawarchi") }
func SpecDir() string                  { return filepath.Join(Dir(), "specs") }
func SrcDir() string                   { return filepath.Join(Dir(), "src") }
func BinDir() string                   { return filepath.Join(Dir(), "bin") }
func registryFile() string             { return filepath.Join(Dir(), "registry.json") }
func specCacheFile(name string) string { return filepath.Join(SpecDir(), name+".spec") }

// CacheSpec stores the raw spec bytes for a CLI under SpecDir (owner-only) so a
// later 'bawarchi update' can fall back to it when the original source has
// moved or the network is unavailable.
func CacheSpec(name string, data []byte) error {
	if err := os.MkdirAll(SpecDir(), 0700); err != nil {
		return err
	}
	return os.WriteFile(specCacheFile(name), data, 0600)
}

// CachedSpec returns the cached raw spec bytes for a CLI, if one was stored.
func CachedSpec(name string) ([]byte, error) {
	return os.ReadFile(specCacheFile(name))
}

// RemoveCachedSpec deletes a CLI's cached spec, ignoring a missing file.
func RemoveCachedSpec(name string) error {
	err := os.Remove(specCacheFile(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func Load() ([]Entry, error) {
	data, err := os.ReadFile(registryFile())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []Entry
	return entries, json.Unmarshal(data, &entries)
}

func Save(entries []Entry) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically: create a temp file in the same directory (same
	// filesystem) and rename it into place so a crash mid-write cannot
	// leave a partially-written registry.json.
	tmp, err := os.CreateTemp(Dir(), ".registry-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after successful rename; cleans up on error
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), registryFile())
}

func Add(entry Entry) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name, entry.Name) {
			return errors.New("already exists — use 'bawarchi update' to regenerate")
		}
	}
	return Save(append(entries, entry))
}

func Get(name string) (*Entry, error) {
	entries, err := Load()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if strings.EqualFold(entries[i].Name, name) {
			return &entries[i], nil
		}
	}
	return nil, errors.New("not found: " + name)
}

func Update(name, specSource, baseURL string) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	for i := range entries {
		if strings.EqualFold(entries[i].Name, name) {
			if specSource != "" {
				entries[i].SpecSource = specSource
			}
			if baseURL != "" {
				entries[i].BaseURL = baseURL
			}
			entries[i].UpdatedAt = time.Now()
			return Save(entries)
		}
	}
	return errors.New("not found: " + name)
}

// SetInstallPath records the symlink path created by 'bawarchi install' so
// 'bawarchi remove' can clean it up automatically.
func SetInstallPath(name, installPath string) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	for i := range entries {
		if strings.EqualFold(entries[i].Name, name) {
			entries[i].InstallPath = installPath
			return Save(entries)
		}
	}
	return errors.New("not found: " + name)
}

func Remove(name string) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	filtered := entries[:0]
	found := false
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) {
			found = true
		} else {
			filtered = append(filtered, e)
		}
	}
	if !found {
		return errors.New("not found: " + name)
	}
	return Save(filtered)
}
