package generator

import (
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

func TestGoMod_GRPCHasNoPhantomRequires(t *testing.T) {
	grpc := goMod(&parser.CLIData{Name: "svc", Transport: parser.TransportGRPC})
	if strings.Contains(string(grpc), "require") {
		t.Errorf("gRPC go.mod must not declare module requires (shells out to grpcurl binary):\n%s", grpc)
	}
	if strings.Contains(string(grpc), "grpcurl") || strings.Contains(string(grpc), "google.golang.org/grpc") {
		t.Errorf("gRPC go.mod must not reference grpc/grpcurl modules:\n%s", grpc)
	}
}

// genFor renders a minimal single-server REST spec with the supplied query and
// body params and returns the generated source.
func genFor(t *testing.T, data *parser.CLIData) string {
	t.Helper()
	src, err := generateREST(data)
	if err != nil {
		t.Fatalf("generateREST: %v", err)
	}
	return string(src)
}

func TestREST_ExitCodesDistinguish4xx5xx(t *testing.T) {
	src := genFor(t, &parser.CLIData{
		Name:     "api",
		Commands: []parser.CommandData{{Name: "x", GoName: "X", Operations: []parser.OperationData{{Name: "get", GoName: "Get", Method: "GET", Path: "/x"}}}},
	})
	if !strings.Contains(src, "os.Exit(4)") {
		t.Error("generated REST CLI should exit 4 on client errors")
	}
	if !strings.Contains(src, "os.Exit(5)") {
		t.Error("generated REST CLI should exit 5 on server errors")
	}
}

func TestREST_ArrayBodyUsesTypedConverter(t *testing.T) {
	data := &parser.CLIData{
		Name:           "api",
		HasBodyParams:  true,
		HasArrayParams: true,
		Commands: []parser.CommandData{{
			Name: "x", GoName: "X",
			Operations: []parser.OperationData{{
				Name: "create", GoName: "Create", Method: "POST", Path: "/x",
				BodyParams: []parser.ParamData{
					{Name: "ids", GoVarName: "Ids", FlagName: "ids", GoType: "string", FlagFunc: "StringVar", DefaultLiteral: `""`, DefaultCmp: `!= ""`, IsArray: true, ElemType: "int"},
					{Name: "tags", GoVarName: "Tags", FlagName: "tags", GoType: "string", FlagFunc: "StringVar", DefaultLiteral: `""`, DefaultCmp: `!= ""`, IsArray: true, ElemType: "string"},
				},
			}},
		}},
	}
	src := genFor(t, data)
	if !strings.Contains(src, `reqBody["ids"] = toIntList(pIds)`) {
		t.Error("int array body should use toIntList")
	}
	if !strings.Contains(src, `reqBody["tags"] = splitList(pTags)`) {
		t.Error("string array body should use splitList")
	}
	if !strings.Contains(src, `"strconv"`) {
		t.Error("array params should pull in the strconv import")
	}
}

func TestREST_RequiredQueryParamValidated(t *testing.T) {
	data := &parser.CLIData{
		Name: "api",
		Commands: []parser.CommandData{{
			Name: "x", GoName: "X",
			Operations: []parser.OperationData{{
				Name: "list", GoName: "List", Method: "GET", Path: "/x",
				QueryParams: []parser.ParamData{
					{Name: "status", GoVarName: "Status", FlagName: "status", GoType: "string", FlagFunc: "StringVar", DefaultLiteral: `""`, DefaultCmp: `!= ""`, ZeroCmp: `== ""`, Required: true},
				},
			}},
		}},
	}
	src := genFor(t, data)
	if !strings.Contains(src, `error: --status is required`) {
		t.Error("required query param should be validated in generated CLI")
	}
}

func TestREST_MultiServerSelectionBlock(t *testing.T) {
	data := &parser.CLIData{
		Name:          "api",
		BaseURL:       "https://a.example.com",
		BaseURLEnvVar: "API__BASE_URL",
		ServerEnvVar:  "API__SERVER",
		Servers: []parser.ServerData{
			{URL: "https://a.example.com", Description: "Prod"},
			{URL: "https://b.example.com", Description: "Staging"},
		},
		Commands: []parser.CommandData{{Name: "x", GoName: "X", Operations: []parser.OperationData{{Name: "get", GoName: "Get", Method: "GET", Path: "/x"}}}},
	}
	src := genFor(t, data)
	if !strings.Contains(src, `os.Getenv("API__SERVER")`) {
		t.Error("multi-server spec should read the server-select env var")
	}
	if !strings.Contains(src, "strconv.Atoi") {
		t.Error("multi-server selection should parse the index with strconv.Atoi")
	}
	if !strings.Contains(src, `"https://b.example.com"`) {
		t.Error("generated servers slice should include all server URLs")
	}
}
