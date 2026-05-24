package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd_HasVersionFlag(t *testing.T) {
	root := rootCmd()
	if root.Version == "" {
		t.Fatal("root command must set a Version so cobra wires up --version")
	}
	if f := root.Flags().Lookup("version"); f == nil {
		// cobra registers the version flag lazily on first use; force it.
		root.InitDefaultVersionFlag()
		if f := root.Flags().Lookup("version"); f == nil {
			t.Fatal("--version flag not registered")
		}
	}
}

func TestRootCmd_VersionOutput(t *testing.T) {
	root := rootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute --version: %v", err)
	}
	if !strings.Contains(buf.String(), "bawarchi") {
		t.Errorf("--version output = %q, want it to contain 'bawarchi'", buf.String())
	}
}
