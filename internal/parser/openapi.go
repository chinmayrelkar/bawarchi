package parser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// OpenAPI 3.x structs only.

type openAPI3Spec struct {
	OpenAPI    string                `yaml:"openapi" json:"openapi"`
	Info       oaInfo                `yaml:"info" json:"info"`
	Servers    []oaServer            `yaml:"servers" json:"servers"`
	Paths      map[string]oaPathItem `yaml:"paths" json:"paths"`
	Components struct {
		SecuritySchemes map[string]oaSecurityScheme `yaml:"securitySchemes" json:"securitySchemes"`
	} `yaml:"components" json:"components"`
}

type oaInfo struct {
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
}

type oaServer struct {
	URL string `yaml:"url" json:"url"`
}

type oaPathItem struct {
	Get    *oaOperation `yaml:"get" json:"get"`
	Post   *oaOperation `yaml:"post" json:"post"`
	Put    *oaOperation `yaml:"put" json:"put"`
	Patch  *oaOperation `yaml:"patch" json:"patch"`
	Delete *oaOperation `yaml:"delete" json:"delete"`
}

type oaOperation struct {
	OperationID string        `yaml:"operationId" json:"operationId"`
	Summary     string        `yaml:"summary" json:"summary"`
	Description string        `yaml:"description" json:"description"`
	Tags        []string      `yaml:"tags" json:"tags"`
	Parameters  []oaParameter `yaml:"parameters" json:"parameters"`
}

// OpenAPI 3.x: type lives inside schema.
type oaParameter struct {
	Name        string `yaml:"name" json:"name"`
	In          string `yaml:"in" json:"in"`
	Description string `yaml:"description" json:"description"`
	Required    bool   `yaml:"required" json:"required"`
	Schema      struct {
		Type string `yaml:"type" json:"type"`
	} `yaml:"schema" json:"schema"`
}

type oaSecurityScheme struct {
	Type   string `yaml:"type" json:"type"`
	In     string `yaml:"in" json:"in"`
	Name   string `yaml:"name" json:"name"`
	Scheme string `yaml:"scheme" json:"scheme"`
}

// ParseOpenAPI parses an OpenAPI 3.x spec.
func ParseOpenAPI(data []byte) (*CLIData, error) {
	var spec openAPI3Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		if err2 := json.Unmarshal(data, &spec); err2 != nil {
			return nil, fmt.Errorf("parsing OpenAPI 3.x spec: %w", err)
		}
	}
	if spec.Info.Title == "" {
		return nil, fmt.Errorf("spec has no info.title")
	}

	cli := &CLIData{
		Name:        toCommandName(spec.Info.Title),
		Description: firstNonEmpty(spec.Info.Description, spec.Info.Title),
		Transport:   TransportREST,
	}

	if len(spec.Servers) > 0 {
		rawURL := strings.TrimRight(spec.Servers[0].URL, "/")
		cli.BaseURL = rawURL
		cli.BaseURLEnvVar = strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(cli.Name, "_")) + "__BASE_URL"
	}

	cli.AuthEnvVar, cli.AuthSetup = authFromSchemes(spec.Components.SecuritySchemes, cli.Name)
	if strings.Contains(cli.AuthSetup, "base64") {
		cli.AuthImport = `"encoding/base64"`
	}
	buildCommandsFromOps(cli, spec.Paths, func(p oaParameter) (in, typ string) {
		return p.In, p.Schema.Type
	})

	return cli, nil
}

// authFromSchemes derives auth env var and Go code snippet from OpenAPI 3.x security schemes.
func authFromSchemes(schemes map[string]oaSecurityScheme, apiName string) (envVar, setup string) {
	prefix := strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(apiName, "_"))
	envVar = prefix + "__API_KEY"

	for schemeName, s := range schemes {
		switch s.Type {
		case "http":
			if strings.EqualFold(s.Scheme, "basic") {
				// key should be "email:apikey" — base64 encoded for Basic auth
				// Caller must set AuthImport = "encoding/base64"
				return prefix + "__CREDENTIALS",
					`req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key)))`
			}
			if strings.EqualFold(s.Scheme, "bearer") {
				return prefix + "__TOKEN", `req.Header.Set("Authorization", "Bearer "+key)`
			}
		case "apiKey":
			if s.In == "header" {
				h := firstNonEmpty(s.Name, "Authorization")
				if strings.EqualFold(h, "authorization") {
					// Authorization header always needs a scheme prefix.
					// Use the security scheme name (e.g. "GenieKey", "Bearer").
					return envVar, fmt.Sprintf(`req.Header.Set("Authorization", %q+" "+key)`, schemeName)
				}
				return envVar, fmt.Sprintf(`req.Header.Set(%q, key)`, h)
			}
		}
	}
	return envVar, `req.Header.Set("Authorization", "Bearer "+key)`
}

// buildCommandsFromOps populates cli.Commands from collected path operations.
func buildCommandsFromOps(cli *CLIData, paths map[string]oaPathItem, getParamInfo func(oaParameter) (in, typ string)) {
	tagOps := map[string][]pathOp{}
	var tagOrder []string

	httpMethods := []struct {
		name string
		get  func(oaPathItem) *oaOperation
	}{
		{"GET", func(p oaPathItem) *oaOperation { return p.Get }},
		{"POST", func(p oaPathItem) *oaOperation { return p.Post }},
		{"PUT", func(p oaPathItem) *oaOperation { return p.Put }},
		{"PATCH", func(p oaPathItem) *oaOperation { return p.Patch }},
		{"DELETE", func(p oaPathItem) *oaOperation { return p.Delete }},
	}

	for path, item := range paths {
		for _, m := range httpMethods {
			op := m.get(item)
			if op == nil {
				continue
			}
			tag := "default"
			if len(op.Tags) > 0 {
				tag = op.Tags[0]
			}
			if _, seen := tagOps[tag]; !seen {
				tagOrder = append(tagOrder, tag)
			}
			tagOps[tag] = append(tagOps[tag], pathOp{method: m.name, path: path, op: op})
		}
	}

	for _, tag := range tagOrder {
		cmd := CommandData{
			Name:        toCommandName(tag),
			GoName:      toPascalCase(tag),
			Description: tag,
		}
		for _, pop := range tagOps[tag] {
			opName := operationName(pop)
			od := OperationData{
				Name:        opName,
				GoName:      toPascalCase(opName),
				Description: firstNonEmpty(pop.op.Summary, pop.op.Description, opName),
				Method:      pop.method,
				Path:        pop.path,
			}
			for _, p := range pop.op.Parameters {
				in, typ := getParamInfo(p)
				pd := makeParamData(p.Name, p.Description, p.Required, typ)
				switch in {
				case "path":
					pd.PathPlaceholder = "{" + p.Name + "}"
					od.PathParams = append(od.PathParams, pd)
					cli.HasPathParams = true
				case "query":
					od.QueryParams = append(od.QueryParams, pd)
				}
			}
			cmd.Operations = append(cmd.Operations, od)
		}
		if len(cmd.Operations) > 0 {
			cli.Commands = append(cli.Commands, cmd)
		}
	}
}

type pathOp struct {
	method string
	path   string
	op     *oaOperation
}

func operationName(pop pathOp) string {
	if pop.op.OperationID != "" {
		return toCommandName(pop.op.OperationID)
	}
	return strings.ToLower(pop.method)
}

func makeParamData(name, description string, required bool, typ string) ParamData {
	pd := ParamData{
		Name:        name,
		GoVarName:   toPascalCase(name),
		FlagName:    toKebabCase(name),
		Description: description,
		Required:    required,
	}
	switch typ {
	case "integer":
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp = "int", "IntVar", "0", "!= 0"
	case "number":
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp = "float64", "Float64Var", "0.0", "!= 0.0"
	default:
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp = "string", "StringVar", `""`, `!= ""`
	}
	return pd
}

// --- string helpers ---

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func toCommandName(s string) string {
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return strings.ToLower(s)
}

func toKebabCase(s string) string {
	re := regexp.MustCompile(`([a-z])([A-Z])`)
	s = re.ReplaceAllString(s, "$1-$2")
	return strings.ToLower(nonAlnum.ReplaceAllString(s, "-"))
}

func toPascalCase(s string) string {
	parts := regexp.MustCompile(`[^a-zA-Z0-9]+`).Split(s, -1)
	var b strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			b.WriteString(p[1:])
		}
	}
	return b.String()
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
