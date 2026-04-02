package parser

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Transport indicates the wire protocol for the generated CLI.
type Transport string

const (
	TransportREST Transport = "rest"
	TransportGRPC Transport = "grpc"
)

// CLIData is the unified representation produced by any parser and consumed by any generator.
type CLIData struct {
	Name           string
	Description    string
	BaseURL        string // REST: https://host/base, gRPC: host:port
	BaseURLEnvVar  string // env var that overrides BaseURL at runtime (e.g. ZULIP__BASE_URL)
	Transport      Transport
	AuthEnvVar     string
	AuthSetup      string // Go code snippet inserted into the generated HTTP/gRPC auth block
	AuthImport     string // extra import needed by AuthSetup (e.g. "encoding/base64")
	HasPathParams  bool
	Commands       []CommandData
}

type CommandData struct {
	Name        string
	GoName      string // PascalCase
	Description string
	Operations  []OperationData
}

type OperationData struct {
	Name        string
	GoName      string // PascalCase
	Description string

	// REST
	Method      string
	Path        string
	PathParams  []ParamData
	QueryParams []ParamData

	// gRPC
	GRPCService string
	GRPCMethod  string
	InputParams []ParamData
}

type ParamData struct {
	Name            string
	GoVarName       string // PascalCase, used as variable suffix (p<GoVarName>)
	FlagName        string // kebab-case CLI flag
	GoType          string // string | int | float64
	FlagFunc        string // StringVar | IntVar | Float64Var
	DefaultLiteral  string // Go literal: "" | 0 | 0.0
	DefaultCmp      string // comparison: != "" | != 0 | != 0.0
	Description     string
	Required        bool
	PathPlaceholder string // "{paramname}" for path substitution
}

// ParseSource loads a spec from a file path or HTTPS URL and dispatches to the right parser.
func ParseSource(source string) (*CLIData, error) {
	data, err := load(source)
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(strings.ToLower(source), ".proto") {
		return ParseProto(data, source)
	}

	switch versionFromBytes(data) {
	case "swagger2":
		return ParseSwagger(data)
	default:
		return ParseOpenAPI(data)
	}
}

func load(source string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source) //nolint:gosec
		if err != nil {
			return nil, fmt.Errorf("fetching spec: %w", err)
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(source)
}
