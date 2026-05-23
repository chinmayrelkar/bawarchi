package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
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
	OperationID string          `yaml:"operationId" json:"operationId"`
	Summary     string          `yaml:"summary" json:"summary"`
	Description string          `yaml:"description" json:"description"`
	Tags        []string        `yaml:"tags" json:"tags"`
	Parameters  []oaParameter   `yaml:"parameters" json:"parameters"`
	RequestBody *oaRequestBody  `yaml:"requestBody" json:"requestBody"`
}

// oaRequestBody represents an OpenAPI 3.x requestBody object.
type oaRequestBody struct {
	Required bool                      `yaml:"required" json:"required"`
	Content  map[string]oaMediaType    `yaml:"content" json:"content"`
}

type oaMediaType struct {
	Schema oaBodySchema `yaml:"schema" json:"schema"`
}

// oaBodySchema covers inline object schemas and $ref.
type oaBodySchema struct {
	Type       string                      `yaml:"type" json:"type"`
	Ref        string                      `yaml:"$ref" json:"$ref"`
	Properties map[string]oaSchemaProp     `yaml:"properties" json:"properties"`
}

type oaSchemaProp struct {
	Type        string `yaml:"type" json:"type"`
	Description string `yaml:"description" json:"description"`
}

// oaParameter represents a parameter in both Swagger 2.0 and OpenAPI 3.x specs.
// In Swagger 2.0, type/format live directly on the parameter object (Type, Format fields).
// In OpenAPI 3.x, type lives inside schema (Schema.Type).
type oaParameter struct {
	Name        string `yaml:"name" json:"name"`
	In          string `yaml:"in" json:"in"`
	Description string `yaml:"description" json:"description"`
	Required    bool   `yaml:"required" json:"required"`
	Type        string `yaml:"type" json:"type"`
	Format      string `yaml:"format" json:"format"`
	Schema      struct {
		Type       string                  `yaml:"type" json:"type"`
		Ref        string                  `yaml:"$ref" json:"$ref"`
		Properties map[string]oaSchemaProp `yaml:"properties" json:"properties"`
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
		Name:        ToCommandName(spec.Info.Title),
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

	if len(cli.Commands) == 0 {
		return nil, fmt.Errorf("spec defines no operations")
	}

	return cli, nil
}

// authFromSchemes derives auth env var and Go code snippet from OpenAPI 3.x security schemes.
//
// Priority order (highest wins):
//
//	1 – http/bearer
//	2 – http/basic
//	3 – apiKey/header
//	4 – apiKey/query
//
// Iterating over a sorted slice of scheme names ensures the selection is
// deterministic regardless of Go's map-iteration order.
func authFromSchemes(schemes map[string]oaSecurityScheme, apiName string) (envVar, setup string) {
	prefix := strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(apiName, "_"))
	envVar = prefix + "__API_KEY"

	// Priority constants – lower value wins.
	const (
		prioBearer   = 1
		prioBasic    = 2
		prioAPIKeyH  = 3
		prioAPIKeyQ  = 4
		prioNone     = 99
	)

	type candidate struct {
		prio       int
		envVar     string
		setup      string
		schemeName string
	}
	best := candidate{prio: prioNone}

	// Collect and sort scheme names for deterministic iteration.
	names := make([]string, 0, len(schemes))
	for n := range schemes {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, schemeName := range names {
		s := schemes[schemeName]
		switch s.Type {
		case "http":
			if strings.EqualFold(s.Scheme, "bearer") && prioBearer < best.prio {
				best = candidate{
					prio:       prioBearer,
					envVar:     prefix + "__TOKEN",
					setup:      `req.Header.Set("Authorization", "Bearer "+key)`,
					schemeName: schemeName,
				}
			}
			if strings.EqualFold(s.Scheme, "basic") && prioBasic < best.prio {
				// key should be "email:apikey" — base64 encoded for Basic auth
				// Caller must set AuthImport = "encoding/base64"
				best = candidate{
					prio:       prioBasic,
					envVar:     prefix + "__CREDENTIALS",
					setup:      `req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(key)))`,
					schemeName: schemeName,
				}
			}
		case "apiKey":
			if s.In == "header" && prioAPIKeyH < best.prio {
				h := firstNonEmpty(s.Name, "Authorization")
				var s string
				if strings.EqualFold(h, "authorization") {
					// Authorization header always needs a scheme prefix.
					// Use the winning security scheme name (e.g. "GenieKey", "Bearer").
					s = fmt.Sprintf(`req.Header.Set("Authorization", %q+" "+key)`, schemeName)
				} else {
					s = fmt.Sprintf(`req.Header.Set(%q, key)`, h)
				}
				best = candidate{
					prio:       prioAPIKeyH,
					envVar:     envVar,
					setup:      s,
					schemeName: schemeName,
				}
			}
			if s.In == "query" && prioAPIKeyQ < best.prio {
				// The spec expects the key as a URL query param, but credentials in
				// URLs are exposed in server logs, browser history, and referrer
				// headers. Pass the key via the Authorization header instead.
				best = candidate{
					prio:       prioAPIKeyQ,
					envVar:     envVar,
					setup:      `req.Header.Set("Authorization", "Bearer "+key)`,
					schemeName: schemeName,
				}
			}
		}
	}

	if best.prio < prioNone {
		fmt.Fprintf(os.Stderr, "bawarchi: using security scheme %q for authentication\n", best.schemeName)
		return best.envVar, best.setup
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

	// Collect sorted path keys first so operation order is deterministic.
	pathKeys := make([]string, 0, len(paths))
	for p := range paths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	for _, path := range pathKeys {
		item := paths[path]
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
	// Sort tags alphabetically so command order is deterministic.
	sort.Strings(tagOrder)

	for _, tag := range tagOrder {
		cmd := CommandData{
			Name:        ToCommandName(tag),
			GoName:      toPascalCase(tag),
			Description: tag,
		}
		seenNames := map[string]int{} // dedup within this command/tag
		for _, pop := range tagOps[tag] {
			opName := operationName(pop)
			if count, exists := seenNames[opName]; exists {
				// Disambiguate by appending a counter suffix.
				seenNames[opName] = count + 1
				opName = fmt.Sprintf("%s-%d", opName, count+1)
			} else {
				seenNames[opName] = 1
			}
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
				case "header":
					od.HeaderParams = append(od.HeaderParams, pd)
				}
			}
			// OpenAPI 3.x requestBody: extract inline application/json properties.
			if pop.op.RequestBody != nil {
				if mt, ok := pop.op.RequestBody.Content["application/json"]; ok {
					if mt.Schema.Ref != "" {
						fmt.Fprintf(os.Stderr, "warning: requestBody uses $ref (%q); body params not yet supported — skipping\n", mt.Schema.Ref)
					} else {
						propNames := make([]string, 0, len(mt.Schema.Properties))
						for n := range mt.Schema.Properties {
							propNames = append(propNames, n)
						}
						sort.Strings(propNames)
						for _, propName := range propNames {
							prop := mt.Schema.Properties[propName]
							od.BodyParams = append(od.BodyParams, makeParamData(propName, prop.Description, false, prop.Type))
						}
						if len(od.BodyParams) > 0 {
							cli.HasBodyParams = true
						}
					}
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

// toCommandName is the private variant of ToCommandName that additionally
// returns a non-nil error when the result would be empty (e.g. operationId "---").
// It is used only for operationId validation; the exported ToCommandName is unchanged.
func toCommandName(s string) (string, error) {
	result := ToCommandName(s)
	if result == "" {
		return "", fmt.Errorf("operationId %q normalizes to an empty string", s)
	}
	return result, nil
}

// methodPathSlug builds a stable operation name from an HTTP method and path,
// e.g. GET /users/{id} → "get-users-id".
// If the path contributes nothing (e.g. "/" or all non-alnum chars), the
// function falls back to just the lower-case method name so the result is
// never empty and never ends with a dangling dash.
func methodPathSlug(method, path string) string {
	// ToCommandName already replaces non-alnum with "-" and trims.
	// Prefix with lower-case method and a separator, then re-clean.
	combined := strings.ToLower(method) + "/" + path
	slug := ToCommandName(combined)
	if slug == "" {
		return strings.ToLower(method)
	}
	return slug
}

func operationName(pop pathOp) string {
	if pop.op.OperationID != "" {
		name, err := toCommandName(pop.op.OperationID)
		if err != nil {
			// operationId normalizes to empty; fall back to method+path slug.
			return methodPathSlug(pop.method, pop.path)
		}
		return name
	}
	return methodPathSlug(pop.method, pop.path)
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

func ToCommandName(s string) string {
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
