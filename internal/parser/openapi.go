package parser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Minimal OpenAPI 3.x structs — only the fields bawarchi needs.

type openAPISpec struct {
	Info       oaInfo                 `yaml:"info" json:"info"`
	Servers    []oaServer             `yaml:"servers" json:"servers"`
	Paths      map[string]oaPathItem  `yaml:"paths" json:"paths"`
	Components oaComponents           `yaml:"components" json:"components"`
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

type oaParameter struct {
	Name        string   `yaml:"name" json:"name"`
	In          string   `yaml:"in" json:"in"`
	Description string   `yaml:"description" json:"description"`
	Required    bool     `yaml:"required" json:"required"`
	Schema      oaSchema `yaml:"schema" json:"schema"`
}

type oaSchema struct {
	Type string `yaml:"type" json:"type"`
}

type oaComponents struct {
	SecuritySchemes map[string]oaSecurityScheme `yaml:"securitySchemes" json:"securitySchemes"`
}

type oaSecurityScheme struct {
	Type   string `yaml:"type" json:"type"`   // apiKey, http
	In     string `yaml:"in" json:"in"`       // header, query, cookie
	Name   string `yaml:"name" json:"name"`   // header or query param name
	Scheme string `yaml:"scheme" json:"scheme"` // bearer, basic
}

type pathOp struct {
	method string
	path   string
	op     *oaOperation
}

func ParseOpenAPI(data []byte, source string) (*CLIData, error) {
	var spec openAPISpec

	// Try YAML first (superset of JSON)
	if err := yaml.Unmarshal(data, &spec); err != nil {
		if err2 := json.Unmarshal(data, &spec); err2 != nil {
			return nil, fmt.Errorf("parsing spec: %w", err)
		}
	}

	if spec.Info.Title == "" {
		return nil, fmt.Errorf("spec has no info.title")
	}

	cli := &CLIData{
		Name:        toCommandName(spec.Info.Title),
		Description: spec.Info.Description,
		Transport:   TransportREST,
	}
	if cli.Description == "" {
		cli.Description = spec.Info.Title
	}

	// Base URL
	if len(spec.Servers) > 0 {
		cli.BaseURL = strings.TrimRight(spec.Servers[0].URL, "/")
	}

	// Auth
	cli.AuthEnvVar, cli.AuthSetup = extractAuth(spec, cli.Name)

	// Group operations by first tag
	tagOps := map[string][]pathOp{}
	tagOrder := []string{}

	methods := []struct {
		name string
		get  func(oaPathItem) *oaOperation
	}{
		{"GET", func(p oaPathItem) *oaOperation { return p.Get }},
		{"POST", func(p oaPathItem) *oaOperation { return p.Post }},
		{"PUT", func(p oaPathItem) *oaOperation { return p.Put }},
		{"PATCH", func(p oaPathItem) *oaOperation { return p.Patch }},
		{"DELETE", func(p oaPathItem) *oaOperation { return p.Delete }},
	}

	for path, item := range spec.Paths {
		for _, m := range methods {
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
		ops := tagOps[tag]
		cmd := CommandData{
			Name:        toCommandName(tag),
			GoName:      toPascalCase(tag),
			Description: tag,
		}

		for _, pop := range ops {
			opName := operationName(pop)
			od := OperationData{
				Name:        opName,
				GoName:      toPascalCase(opName),
				Description: firstNonEmpty(pop.op.Summary, pop.op.Description, opName),
				Method:      pop.method,
				Path:        pop.path,
			}

			for _, p := range pop.op.Parameters {
				pd := buildParamData(p)
				switch p.In {
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

	return cli, nil
}

func extractAuth(spec openAPISpec, apiName string) (envVar, setup string) {
	prefix := strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(apiName, "_"))
	envVar = prefix + "_API_KEY"

	for _, scheme := range spec.Components.SecuritySchemes {
		switch scheme.Type {
		case "http":
			if strings.EqualFold(scheme.Scheme, "bearer") {
				envVar = prefix + "_TOKEN"
				setup = `req.Header.Set("Authorization", "Bearer "+key)`
				return
			}
		case "apiKey":
			if scheme.In == "header" {
				headerName := scheme.Name
				if headerName == "" {
					headerName = "Authorization"
				}
				setup = fmt.Sprintf(`req.Header.Set(%q, key)`, headerName)
				return
			}
		}
	}

	// Default: assume Bearer token
	setup = `req.Header.Set("Authorization", "Bearer "+key)`
	return
}

func buildParamData(p oaParameter) ParamData {
	pd := ParamData{
		Name:        p.Name,
		GoVarName:   toPascalCase(p.Name),
		FlagName:    toKebabCase(p.Name),
		Description: p.Description,
		Required:    p.Required,
	}

	switch p.Schema.Type {
	case "integer":
		pd.GoType = "int"
		pd.FlagFunc = "IntVar"
		pd.DefaultLiteral = "0"
		pd.DefaultCmp = "!= 0"
	case "number":
		pd.GoType = "float64"
		pd.FlagFunc = "Float64Var"
		pd.DefaultLiteral = "0.0"
		pd.DefaultCmp = "!= 0.0"
	default:
		pd.GoType = "string"
		pd.FlagFunc = "StringVar"
		pd.DefaultLiteral = `""`
		pd.DefaultCmp = `!= ""`
	}

	return pd
}

func operationName(pop pathOp) string {
	if pop.op.OperationID != "" {
		return toCommandName(pop.op.OperationID)
	}
	// fallback: method name
	return strings.ToLower(pop.method)
}

// --- string helpers ---

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func toCommandName(s string) string {
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return strings.ToLower(s)
}

func toKebabCase(s string) string {
	// insert hyphen before uppercase letters (camelCase → kebab-case)
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
