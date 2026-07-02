// Package compiler builds a generated CLI's Go source into a binary.
package compiler

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

// Compile runs `go build` in srcDir and writes the binary to outBin.
//
// The `go` toolchain's stdout/stderr are captured into a buffer and surfaced
// only when a step fails, so successful builds stay quiet and don't pollute the
// caller's streams with toolchain progress noise.
func Compile(srcDir, outBin string) error {
	if err := os.MkdirAll(srcDir, 0700); err != nil {
		return err
	}

	// go mod tidy first (resolves deps for generated code)
	if err := runQuiet(srcDir, "go", "mod", "tidy"); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	if err := runQuiet(srcDir, "go", "build", "-o", outBin, "."); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	return nil
}

// runQuiet runs a command in dir, discarding its output on success and
// attaching the captured combined output to the error on failure.
func runQuiet(dir, name string, args ...string) error {
	var out bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		if out.Len() > 0 {
			return fmt.Errorf("%w\n%s", err, out.String())
		}
		return err
	}
	return nil
}
