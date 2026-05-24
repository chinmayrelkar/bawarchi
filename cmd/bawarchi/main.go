package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/chinmayrelkar/bawarchi/internal/compiler"
	"github.com/chinmayrelkar/bawarchi/internal/generator"
	"github.com/chinmayrelkar/bawarchi/internal/parser"
	"github.com/chinmayrelkar/bawarchi/internal/registry"
	"github.com/spf13/cobra"
)

// version is the bawarchi release version. Override at build time with:
//
//	go build -ldflags "-X main.version=v1.2.3" ./cmd/bawarchi
var version = "dev"

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "bawarchi",
		Short:   "Generate and manage CLIs from OpenAPI specs and proto files",
		Version: version,
	}
	root.SetVersionTemplate("bawarchi {{.Version}}\n")
	root.AddCommand(
		addCmd(),
		listCmd(),
		updateCmd(),
		installCmd(),
		removeCmd(),
		infoCmd(),
	)
	return root
}

// add -------------------------------------------------------------------------

func addCmd() *cobra.Command {
	var name, baseURL string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add <spec>",
		Short: "Generate a CLI from an OpenAPI spec or .proto file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]

			fmt.Printf("Parsing %s…\n", source)
			raw, err := parser.Load(source)
			if err != nil {
				return fmt.Errorf("load: %w", err)
			}
			data, err := parser.ParseBytes(raw, source)
			if err != nil {
				return fmt.Errorf("parse: %w", err)
			}
			if name != "" {
				sanitized, err := sanitizeCLIName(name)
				if err != nil {
					return err
				}
				data.Name = sanitized
			}
			if baseURL != "" {
				data.BaseURL = baseURL
			}

			// --dry-run: print generated source and exit without touching disk or registry.
			if dryRun {
				src, err := generator.GenerateDry(data)
				if err != nil {
					return fmt.Errorf("generate: %w", err)
				}
				fmt.Fprintln(os.Stdout, "--- generated: main.go ---")
				os.Stdout.Write(src)
				return nil
			}

			// Check for duplicates
			if _, err := registry.Get(data.Name); err == nil {
				return fmt.Errorf("%q already exists — use 'bawarchi update %s' to regenerate", data.Name, data.Name)
			}

			if err := cookAndRegister(data, source, baseURL, true); err != nil {
				return err
			}

			// Cache the raw spec so 'bawarchi update' survives a moved or
			// offline source. Non-fatal: the CLI is already built.
			if err := registry.CacheSpec(data.Name, raw); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not cache spec: %v\n", err)
			}

			fmt.Printf("\n✓ %s is ready at %s\n", data.Name, filepath.Join(registry.BinDir(), data.Name))
			fmt.Printf("  Run 'bawarchi install %s' to add it to your PATH\n", data.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Override the CLI name derived from the spec")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override the base URL from the spec (e.g. for EU endpoints)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print generated main.go to stdout without compiling or registering")
	return cmd
}

// list ------------------------------------------------------------------------

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all generated CLIs",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := registry.Load()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No CLIs yet. Run 'bawarchi add <spec>' to cook one.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTRANSPORT\tUPDATED\tSOURCE")
			for _, e := range entries {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					e.Name, e.Transport, e.UpdatedAt.Format("2006-01-02"), e.SpecSource)
			}
			return w.Flush()
		},
	}
}

// update ----------------------------------------------------------------------

func updateCmd() *cobra.Command {
	var source, baseURL string
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Re-fetch the spec and regenerate a CLI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rawName := args[0]
			sanitizedName, err := sanitizeCLIName(rawName)
			if err != nil {
				return err
			}
			entry, err := registry.Get(sanitizedName)
			if err != nil {
				return err
			}

			src := entry.SpecSource
			if source != "" {
				src = source
			}
			overrideURL := entry.BaseURL
			if baseURL != "" {
				overrideURL = baseURL
			}

			fmt.Printf("Re-parsing %s…\n", src)
			raw, err := parser.Load(src)
			if err != nil {
				// Source unreachable — fall back to the cached spec if present.
				cached, cacheErr := registry.CachedSpec(sanitizedName)
				if cacheErr != nil {
					return fmt.Errorf("load %s: %w (no cached spec to fall back on)", src, err)
				}
				fmt.Fprintf(os.Stderr, "warning: could not fetch %s (%v); using cached spec\n", src, err)
				raw = cached
			}
			data, err := parser.ParseBytes(raw, src)
			if err != nil {
				return fmt.Errorf("parse: %w", err)
			}
			data.Name = sanitizedName
			if overrideURL != "" {
				data.BaseURL = overrideURL
			}

			if err := cookAndRegister(data, src, overrideURL, false); err != nil {
				return err
			}

			// Refresh the cache with the freshly fetched spec (no-op on fallback).
			if err := registry.CacheSpec(sanitizedName, raw); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not cache spec: %v\n", err)
			}

			if err := registry.Update(sanitizedName, src, overrideURL); err != nil {
				return fmt.Errorf("updating registry: %w", err)
			}
			fmt.Printf("✓ %s updated\n", sanitizedName)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "Use a different spec source for this update")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Override the base URL (persisted; defaults to stored value)")
	return cmd
}

// install ---------------------------------------------------------------------

func installCmd() *cobra.Command {
	var installDir string
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Symlink a CLI to a directory on your PATH",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := registry.Get(name); err != nil {
				return err
			}

			if installDir == "" {
				installDir = filepath.Join(os.Getenv("HOME"), ".local", "bin")
			}
			if err := os.MkdirAll(installDir, 0755); err != nil {
				return err
			}

			src := filepath.Join(registry.BinDir(), name)
			dst := filepath.Join(installDir, name)
			os.Remove(dst) //nolint:errcheck
			if err := os.Symlink(src, dst); err != nil {
				return err
			}
			// Record the symlink path so 'bawarchi remove' can clean it up.
			if err := registry.SetInstallPath(name, dst); err != nil {
				// Non-fatal: symlink already exists, just couldn't persist the path.
				fmt.Fprintf(os.Stderr, "warning: could not record install path: %v\n", err)
			}

			fmt.Printf("✓ %s → %s\n", dst, src)
			return nil
		},
	}
	cmd.Flags().StringVar(&installDir, "dir", "", "Installation directory (default: ~/.local/bin)")
	return cmd
}

// remove ----------------------------------------------------------------------

func removeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a generated CLI and remove it from the registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			entry, err := registry.Get(name)
			if err != nil {
				return err
			}
			installPath := entry.InstallPath
			if err := registry.Remove(name); err != nil {
				return err
			}
			os.RemoveAll(filepath.Join(registry.SrcDir(), name)) //nolint:errcheck
			os.Remove(filepath.Join(registry.BinDir(), name))    //nolint:errcheck
			registry.RemoveCachedSpec(name)                      //nolint:errcheck
			// Remove the install symlink if one was recorded by 'bawarchi install'.
			if installPath != "" {
				os.Remove(installPath) //nolint:errcheck
			}
			fmt.Printf("✓ %s removed\n", name)
			return nil
		},
	}
}

// info ------------------------------------------------------------------------

func infoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show details about a generated CLI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			e, err := registry.Get(name)
			if err != nil {
				return err
			}
			fmt.Printf("Name:      %s\n", e.Name)
			fmt.Printf("Transport: %s\n", e.Transport)
			fmt.Printf("Spec:      %s\n", e.SpecSource)
			fmt.Printf("Added:     %s\n", e.AddedAt.Format(time.RFC3339))
			fmt.Printf("Updated:   %s\n", e.UpdatedAt.Format(time.RFC3339))
			fmt.Printf("Binary:    %s\n", filepath.Join(registry.BinDir(), e.Name))
			fmt.Printf("Code:      %s\n", filepath.Join(registry.SrcDir(), e.Name))
			return nil
		},
	}
}

// --- helpers -----------------------------------------------------------------

// sanitizeCLIName runs the raw name through parser.ToCommandName to neutralize
// path separators and special characters, then validates the result is safe to
// use as a filesystem path component.
func sanitizeCLIName(raw string) (string, error) {
	name := parser.ToCommandName(raw)
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) {
		return "", fmt.Errorf("invalid CLI name %q", raw)
	}
	return name, nil
}

func cookAndRegister(data *parser.CLIData, source, baseURL string, isNew bool) error {
	srcDir := filepath.Join(registry.SrcDir(), data.Name)
	binDir := registry.BinDir()
	binPath := filepath.Join(binDir, data.Name)

	if err := os.MkdirAll(binDir, 0700); err != nil {
		return err
	}

	fmt.Printf("Generating %s CLI (%s)…\n", data.Transport, data.Name)
	if err := generator.Generate(data, srcDir); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	fmt.Printf("Compiling…\n")
	if err := compiler.Compile(srcDir, binPath); err != nil {
		return fmt.Errorf("compile: %w", err)
	}

	if isNew {
		return registry.Add(registry.Entry{
			Name:       data.Name,
			SpecSource: source,
			Transport:  string(data.Transport),
			BaseURL:    baseURL,
			AddedAt:    time.Now(),
			UpdatedAt:  time.Now(),
		})
	}
	return nil
}
