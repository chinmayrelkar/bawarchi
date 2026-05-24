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
	ServerEnvVar   string // env var that selects a predefined server by index (e.g. ZULIP__SERVER)
	Transport      Transport
	AuthEnvVar     string
	AuthSetup      string // Go code snippet inserted into the generated HTTP/gRPC auth block
	AuthImport     string // extra import needed by AuthSetup (e.g. "encoding/base64")
	HasPathParams  bool
	HasBodyParams  bool         // true when at least one operation carries a JSON request body
	HasArrayParams bool         // true when at least one param is an array (drives strconv import + list helpers)
	Servers        []ServerData // additional named servers for environment selection
	Commands       []CommandData
}

// ServerData describes a selectable server/environment for a multi-server spec.
type ServerData struct {
	URL         string
	Description string
}

// finalize derives whole-CLI flags (e.g. HasArrayParams) from the assembled
// command tree. Call once after all commands are built.
func (c *CLIData) finalize() {
	for _, cmd := range c.Commands {
		for _, op := range cmd.Operations {
			groups := [][]ParamData{op.QueryParams, op.BodyParams, op.PathParams, op.HeaderParams, op.InputParams}
			for _, g := range groups {
				for _, p := range g {
					if p.IsArray {
						c.HasArrayParams = true
					}
				}
			}
		}
	}
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
	Method       string
	Path         string
	PathParams   []ParamData
	QueryParams  []ParamData
	HeaderParams []ParamData // HTTP header fields (in: header)
	BodyParams   []ParamData // JSON request body fields (POST/PUT/PATCH)

	// gRPC
	GRPCService string
	GRPCMethod  string
	InputParams []ParamData
}

type ParamData struct {
	Name            string
	GoVarName       string // PascalCase, used as variable suffix (p<GoVarName>)
	FlagName        string // kebab-case CLI flag
	GoType          string // string | int | float64 | bool
	FlagFunc        string // StringVar | IntVar | Float64Var | BoolVar
	DefaultLiteral  string // Go literal: "" | 0 | 0.0 | false
	DefaultCmp      string // "set if provided" guard: != "" | != 0 | != 0.0 | != false
	ZeroCmp         string // "is missing" check: == "" | == 0 | == 0.0 | == false
	Description     string
	Required        bool
	IsArray         bool   // true: flag is a comma-separated list expanded into repeated/array values
	ElemType        string // array element Go type: string | int | float64 | bool
	PathPlaceholder string // "{paramname}" for path substitution
}

// ParseSource loads a spec from a file path or HTTPS URL and dispatches to the right parser.
func ParseSource(source string) (*CLIData, error) {
	data, err := Load(source)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data, source)
}

// ParseBytes parses already-loaded spec bytes. The source string is used only
// to detect .proto files; for OpenAPI/Swagger the format is sniffed from the
// content. This lets callers cache the raw bytes (see registry.CacheSpec) and
// re-parse them without re-fetching.
func ParseBytes(data []byte, source string) (*CLIData, error) {
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

// Load fetches the raw spec bytes from a local file path or an HTTPS URL.
func Load(source string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") {
		return nil, fmt.Errorf("spec source must use https:// (got http://)")
	}
	if strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source)
		if err != nil {
			return nil, fmt.Errorf("fetching spec: %w", err)
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(source)
}
