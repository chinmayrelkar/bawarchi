package registry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type Entry struct {
	Name        string    `json:"name"`
	SpecSource  string    `json:"spec_source"`
	Transport   string    `json:"transport"` // "rest" or "grpc"
	BaseURL     string    `json:"base_url,omitempty"` // overridden base URL (e.g. EU endpoint)
	AddedAt     time.Time `json:"added_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func Dir() string        { return filepath.Join(home(), ".bawarchi") }
func SpecDir() string    { return filepath.Join(Dir(), "specs") }
func SrcDir() string     { return filepath.Join(Dir(), "src") }
func BinDir() string     { return filepath.Join(Dir(), "bin") }
func registryFile() string { return filepath.Join(Dir(), "registry.json") }

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
	if err := os.MkdirAll(Dir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(registryFile(), data, 0644)
}

func Add(entry Entry) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name == entry.Name {
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
		if entries[i].Name == name {
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
		if entries[i].Name == name {
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

func Remove(name string) error {
	entries, err := Load()
	if err != nil {
		return err
	}
	filtered := entries[:0]
	found := false
	for _, e := range entries {
		if e.Name == name {
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
