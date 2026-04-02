package generator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

// Generate writes Go source + go.mod for the given CLIData into outDir.
func Generate(data *parser.CLIData, outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
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

	if err := os.WriteFile(filepath.Join(outDir, "main.go"), mainSrc, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "go.mod"), modSrc, 0644); err != nil {
		return err
	}

	return nil
}

func goMod(data *parser.CLIData) []byte {
	if data.Transport == parser.TransportGRPC {
		return []byte(`module bawarchi/gen/` + data.Name + `

go 1.21

require (
	github.com/fullstorydev/grpcurl v1.9.1
	google.golang.org/grpc v1.62.0
)
`)
	}
	return []byte(`module bawarchi/gen/` + data.Name + `

go 1.21
`)
}
