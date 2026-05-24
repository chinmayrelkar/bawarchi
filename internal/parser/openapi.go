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
		Schemas         map[string]oaSchema         `yaml:"schemas" json:"schemas"`
		Parameters      map[string]oaParameter      `yaml:"parameters" json:"parameters"`
	} `yaml:"components" json:"components"`
}

type oaInfo struct {
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
}

type oaServer struct {
	URL         string `yaml:"url" json:"url"`
	Description string `yaml:"description" json:"description"`
}

type oaPathItem struct {
	Get    *oaOperation `yaml:"get" json:"get"`
	Post   *oaOperation `yaml:"post" json:"post"`
	Put    *oaOperation `yaml:"put" json:"put"`
	Patch  *oaOperation `yaml:"patch" json:"patch"`
	Delete *oaOperation `yaml:"delete" json:"delete"`
}

type oaOperation struct {
	OperationID string         `yaml:"operationId" json:"operationId"`
	Summary     string         `yaml:"summary" json:"summary"`
	Description string         `yaml:"description" json:"description"`
	Tags        []string       `yaml:"tags" json:"tags"`
	Parameters  []oaParameter  `yaml:"parameters" json:"parameters"`
	RequestBody *oaRequestBody `yaml:"requestBody" json:"requestBody"`
}

// oaRequestBody represents an OpenAPI 3.x requestBody object.
type oaRequestBody struct {
	Required bool                   `yaml:"required" json:"required"`
	Content  map[string]oaMediaType `yaml:"content" json:"content"`
}

type oaMediaType struct {
	Schema oaSchema `yaml:"schema" json:"schema"`
}

// oaSchema covers inline object schemas, arrays, and $ref. It is shared by
// requestBody media types, parameter schemas, and components/schemas entries.
type oaSchema struct {
	Type       string                  `yaml:"type" json:"type"`
	Ref        string                  `yaml:"$ref" json:"$ref"`
	Format     string                  `yaml:"format" json:"format"`
	Properties map[string]oaSchemaProp `yaml:"properties" json:"properties"`
	Required   []string                `yaml:"required" json:"required"`
	Items      *oaItems                `yaml:"items" json:"items"`
}

// oaSchemaProp is a single property within an object schema. A property may
// itself be a scalar, an array (Items), a nested object (Properties), or a
// $ref to another schema (Ref).
type oaSchemaProp struct {
	Type        string   `yaml:"type" json:"type"`
	Format      string   `yaml:"format" json:"format"`
	Description string   `yaml:"description" json:"description"`
	Ref         string   `yaml:"$ref" json:"$ref"`
	Items       *oaItems `yaml:"items" json:"items"`
}

// oaItems describes the element type of an array (scalar type or $ref).
type oaItems struct {
	Type string `yaml:"type" json:"type"`
	Ref  string `yaml:"$ref" json:"$ref"`
}

// oaParameter represents a parameter in both Swagger 2.0 and OpenAPI 3.x specs.
// In Swagger 2.0, type/format/items live directly on the parameter object.
// In OpenAPI 3.x, type lives inside Schema. A parameter may also be a $ref
// (OpenAPI components/parameters or Swagger #/parameters), captured by Ref.
type oaParameter struct {
	Ref         string    `yaml:"$ref" json:"$ref"`
	Name        string    `yaml:"name" json:"name"`
	In          string    `yaml:"in" json:"in"`
	Description string    `yaml:"description" json:"description"`
	Required    bool      `yaml:"required" json:"required"`
	Type        string    `yaml:"type" json:"type"`
	Format      string    `yaml:"format" json:"format"`
	Items       *oaItems  `yaml:"items" json:"items"`   // Swagger 2.0 array element type
	Schema      *oaSchema `yaml:"schema" json:"schema"` // OpenAPI 3.x / Swagger body schema
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
		prefix := strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(cli.Name, "_"))
		cli.BaseURL = strings.TrimRight(spec.Servers[0].URL, "/")
		cli.BaseURLEnvVar = prefix + "__BASE_URL"
		cli.ServerEnvVar = prefix + "__SERVER"
		for _, s := range spec.Servers {
			cli.Servers = append(cli.Servers, ServerData{
				URL:         strings.TrimRight(s.URL, "/"),
				Description: s.Description,
			})
		}
	}

	cli.AuthEnvVar, cli.AuthSetup = authFromSchemes(spec.Components.SecuritySchemes, cli.Name)
	if strings.Contains(cli.AuthSetup, "base64") {
		cli.AuthImport = `"encoding/base64"`
	}
	buildCommandsFromOps(cli, &spec)

	if len(cli.Commands) == 0 {
		return nil, fmt.Errorf("spec defines no operations")
	}

	cli.finalize()
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
		prioBearer  = 1
		prioBasic   = 2
		prioAPIKeyH = 3
		prioAPIKeyQ = 4
		prioNone    = 99
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

// buildCommandsFromOps populates cli.Commands from collected path operations,
// resolving parameter and requestBody $refs against the spec's components.
func buildCommandsFromOps(cli *CLIData, spec *openAPI3Spec) {
	paths := spec.Paths
	schemas := spec.Components.Schemas
	paramDefs := spec.Components.Parameters

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
				// Resolve parameter-level $ref (components/parameters).
				if p.Ref != "" {
					if resolved, ok := paramDefs[refName(p.Ref)]; ok {
						p = resolved
					} else {
						fmt.Fprintf(os.Stderr, "warning: cannot resolve parameter $ref %q; skipping\n", p.Ref)
						continue
					}
				}
				typ, itemType := "string", ""
				if p.Schema != nil {
					typ = firstNonEmpty(p.Schema.Type, "string")
					if p.Schema.Items != nil {
						itemType = p.Schema.Items.Type
					}
				}
				pd := makeParamData(p.Name, p.Description, p.Required, typ, itemType)
				switch p.In {
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
			// OpenAPI 3.x requestBody: extract application/json properties,
			// resolving a top-level $ref against components/schemas.
			if pop.op.RequestBody != nil {
				if mt, ok := pop.op.RequestBody.Content["application/json"]; ok {
					od.BodyParams = bodyParamsFromSchema(mt.Schema, schemas)
					if len(od.BodyParams) > 0 {
						cli.HasBodyParams = true
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

// refName returns the final path segment of a JSON-Reference string, e.g.
// "#/components/schemas/Pet" -> "Pet".
func refName(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

// bodyParamsFromSchema flattens an object schema into body flag params,
// resolving a top-level $ref against the supplied schema map. Scalar and array
// properties become typed flags; nested objects / $ref properties become a
// raw-JSON string flag. Ref cycles are guarded against.
func bodyParamsFromSchema(schema oaSchema, schemas map[string]oaSchema) []ParamData {
	return bodyParamsResolve(schema, schemas, map[string]bool{})
}

func bodyParamsResolve(schema oaSchema, schemas map[string]oaSchema, seen map[string]bool) []ParamData {
	if schema.Ref != "" {
		name := refName(schema.Ref)
		if seen[name] {
			return nil // cycle guard
		}
		seen[name] = true
		resolved, ok := schemas[name]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: cannot resolve schema $ref %q; skipping body fields\n", schema.Ref)
			return nil
		}
		return bodyParamsResolve(resolved, schemas, seen)
	}
	// Array body: a top-level array has no named fields to map to flags.
	if schema.Type == "array" {
		fmt.Fprintf(os.Stderr, "warning: top-level array request bodies are not expanded into flags; skipping\n")
		return nil
	}

	reqSet := map[string]bool{}
	for _, r := range schema.Required {
		reqSet[r] = true
	}
	propNames := make([]string, 0, len(schema.Properties))
	for n := range schema.Properties {
		propNames = append(propNames, n)
	}
	sort.Strings(propNames)

	var params []ParamData
	for _, propName := range propNames {
		prop := schema.Properties[propName]
		typ, itemType := prop.Type, ""
		switch {
		case prop.Ref != "":
			// Nested object reference — accept as raw JSON via a string flag.
			typ = "string"
		case prop.Type == "object":
			typ = "string"
		case prop.Type == "array":
			if prop.Items != nil {
				if prop.Items.Ref != "" {
					itemType = "string" // array of objects: comma-separated JSON
				} else {
					itemType = prop.Items.Type
				}
			}
		}
		params = append(params, makeParamData(propName, prop.Description, reqSet[propName], typ, itemType))
	}
	return params
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

func makeParamData(name, description string, required bool, typ, itemType string) ParamData {
	pd := ParamData{
		Name:        name,
		GoVarName:   toPascalCase(name),
		FlagName:    toKebabCase(name),
		Description: description,
		Required:    required,
	}
	applyType(&pd, typ, itemType)
	return pd
}

// applyType maps an OpenAPI/Swagger type onto a ParamData's Go flag fields.
// Arrays are entered as a comma-separated string flag and expanded by the
// generated CLI into repeated query values or a JSON array body field.
func applyType(pd *ParamData, typ, itemType string) {
	switch typ {
	case "integer":
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp, pd.ZeroCmp = "int", "IntVar", "0", "!= 0", "== 0"
	case "number":
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp, pd.ZeroCmp = "float64", "Float64Var", "0.0", "!= 0.0", "== 0.0"
	case "boolean":
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp, pd.ZeroCmp = "bool", "BoolVar", "false", "!= false", "== false"
	case "array":
		pd.IsArray = true
		pd.ElemType = goElemType(itemType)
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp, pd.ZeroCmp = "string", "StringVar", `""`, `!= ""`, `== ""`
		if pd.Description == "" {
			pd.Description = "(comma-separated)"
		} else {
			pd.Description += " (comma-separated)"
		}
	default:
		pd.GoType, pd.FlagFunc, pd.DefaultLiteral, pd.DefaultCmp, pd.ZeroCmp = "string", "StringVar", `""`, `!= ""`, `== ""`
	}
}

// goElemType maps an array item type to the Go scalar used when emitting JSON
// body arrays. Unknown / object element types fall back to string.
func goElemType(itemType string) string {
	switch itemType {
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	default:
		return "string"
	}
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
