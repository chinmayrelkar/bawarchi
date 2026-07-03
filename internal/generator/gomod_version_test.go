package generator

import (
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

// TestGoMod_UsesFixedFloorNotRuntimeVersion verifies the generated go.mod
// pins a fixed, conservative Go version rather than the version bawarchi
// itself happened to be built/run with. A prebuilt bawarchi release binary
// is typically built on CI with a newer Go toolchain than what's installed
// on the machine running `bawarchi add`; stamping that build-time version
// into the generated CLI's go.mod forces an unwanted (and sometimes
// unavailable) toolchain download just to build trivial stdlib-only code.
func TestGoMod_UsesFixedFloorNotRuntimeVersion(t *testing.T) {
	mod := string(goMod(&parser.CLIData{Name: "example"}))

	if !strings.Contains(mod, "go "+minGoVersion+"\n") {
		t.Errorf("go.mod must pin the fixed floor %q, got:\n%s", minGoVersion, mod)
	}
	if !strings.Contains(mod, "module bawarchi/gen/example") {
		t.Errorf("go.mod must declare the module path, got:\n%s", mod)
	}
}
