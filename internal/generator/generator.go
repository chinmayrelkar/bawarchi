// Package generator renders a parser.CLIData into a self-contained Go
// main.go for the target transport (REST or gRPC).
package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

// GenerateDry returns the generated main.go source bytes for the given CLIData
// without writing any files to disk. It is safe to call for specs already in
// the registry. Returns a non-nil error for unknown transports.
func GenerateDry(data *parser.CLIData) ([]byte, error) {
	switch data.Transport {
	case parser.TransportREST:
		return generateREST(data)
	case parser.TransportGRPC:
		return generateGRPC(data)
	default:
		return nil, fmt.Errorf("unknown transport: %s", data.Transport)
	}
}

// Generate writes Go source + go.mod for the given CLIData into outDir.
//
// Generated artifacts may embed internal base URLs and auth env-var names, so
// the directory (0700) and files (0600) are created owner-only. Generate runs
// before the compiler, so it owns the initial permissions of these paths.
func Generate(data *parser.CLIData, outDir string) error {
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return err
	}

	var (
		mainSrc []byte
		modSrc  []byte
		err     error
	)

	switch data.Transport {
	case parser.TransportREST:
		mainSrc, err = generateREST(data)
	case parser.TransportGRPC:
		mainSrc, err = generateGRPC(data)
	default:
		err = fmt.Errorf("unknown transport: %s", data.Transport)
	}
	if err != nil {
		return err
	}

	modSrc = goMod(data)

	if err := os.WriteFile(filepath.Join(outDir, "main.go"), mainSrc, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), modSrc, 0600); err != nil {
		return err
	}

	return nil
}

// goVersion returns the major.minor Go toolchain version (e.g. "1.22") used to
// run bawarchi, so generated go.mod files stay in sync with the host toolchain.
func goVersion() string {
	v := runtime.Version() // e.g. "go1.22.3"
	v = strings.TrimPrefix(v, "go")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return v
}

func goMod(data *parser.CLIData) []byte {
	// Both REST and gRPC generated CLIs import only the standard library: the
	// gRPC CLI shells out to the `grpcurl` binary rather than linking grpc/
	// grpcurl as Go modules. So no require block is needed for either transport,
	// which keeps `go mod tidy` offline-friendly (no phantom downloads).
	return []byte(`module bawarchi/gen/` + data.Name + `

go ` + goVersion() + `
`)
}
