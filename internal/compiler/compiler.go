package compiler

import (
	"fmt"
	"os"
	"os/exec"
)

// Compile runs `go build` in srcDir and writes the binary to outBin.
func Compile(srcDir, outBin string) error {
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return err
	}

	// go mod tidy first (resolves deps for generated code)
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = srcDir
	tidy.Stdout = os.Stderr // progress to stderr
	tidy.Stderr = os.Stderr
	if err := tidy.Run(); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	build := exec.Command("go", "build", "-o", outBin, ".")
	build.Dir = srcDir
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	return nil
}
