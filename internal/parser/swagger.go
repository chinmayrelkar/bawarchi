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

// Swagger 2.0 structs only.

type swagger2Spec struct {
	Swagger             string                      `yaml:"swagger" json:"swagger"`
	Info                oaInfo                      `yaml:"info" json:"info"`
	Host                string                      `yaml:"host" json:"host"`
	BasePath            string                      `yaml:"basePath" json:"basePath"`
	Schemes             []string                    `yaml:"schemes" json:"schemes"`
	Paths               map[string]oaPathItem       `yaml:"paths" json:"paths"`
	SecurityDefinitions map[string]oaSecurityScheme `yaml:"securityDefinitions" json:"securityDefinitions"`
	Definitions         map[string]oaSchema         `yaml:"definitions" json:"definitions"`
	Parameters          map[string]yaml.Node        `yaml:"parameters" json:"parameters"`
}

// ParseSwagger parses a Swagger 2.0 spec.
func ParseSwagger(data []byte) (*CLIData, error) {
	var spec swagger2Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		if err2 := json.Unmarshal(data, &spec); err2 != nil {
			return nil, fmt.Errorf("parsing Swagger 2.0 spec: %w", err)
		}
	}
	if spec.Info.Title == "" {
		return nil, fmt.Errorf("spec has no info.title")
	}

	name := ToCommandName(spec.Info.Title)
	cli := &CLIData{
		Name:          name,
		Description:   firstNonEmpty(spec.Info.Description, spec.Info.Title),
		Transport:     TransportREST,
		BaseURL:       swagger2BaseURL(spec),
		BaseURLEnvVar: strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(name, "_")) + "__BASE_URL",
	}

	cli.AuthEnvVar, cli.AuthSetup = authFromSchemes(spec.SecurityDefinitions, cli.Name)
	if strings.Contains(cli.AuthSetup, "base64") {
		cli.AuthImport = `"encoding/base64"`
	}

	// Re-parse paths with Swagger 2.0 parameter handling.
	// Swagger 2.0 and OpenAPI 3.x share the same path/operation structure
	// except for how parameters encode their type and bodies.
	buildCommandsFromSwagger(cli, &spec)

	if len(cli.Commands) == 0 {
		return nil, fmt.Errorf("spec defines no operations")
	}

	cli.finalize()
	return cli, nil
}

func swagger2BaseURL(spec swagger2Spec) string {
	scheme := "https"
	if len(spec.Schemes) > 0 {
		scheme = "http" // fallback if https not found
		for _, s := range spec.Schemes {
			if s == "https" {
				scheme = "https"
				break
			}
		}
		if scheme == "http" {
			fmt.Fprintln(os.Stderr, "warning: swagger spec does not list 'https'; falling back to http — traffic will not be encrypted")
		}
	}
	base := scheme + "://" + spec.Host
	if spec.BasePath != "" && spec.BasePath != "/" {
		base += strings.TrimRight(spec.BasePath, "/")
	}
	return base
}

// buildCommandsFromSwagger builds CLI commands from Swagger 2.0 paths.
// Swagger 2.0 parameters carry `type` directly on the parameter object
// (oaParameter.Type); body parameters carry an inline or $ref schema. Parameter
// and body $refs are resolved against #/parameters and #/definitions.
func buildCommandsFromSwagger(cli *CLIData, spec *swagger2Spec) {
	rawPaths := spec.Paths
	definitions := spec.Definitions
	paramDefs := resolveParamDefs(spec.Parameters)

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
	pathKeys := make([]string, 0, len(rawPaths))
	for p := range rawPaths {
		pathKeys = append(pathKeys, p)
	}
	sort.Strings(pathKeys)

	for _, path := range pathKeys {
		item := rawPaths[path]
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
			for _, raw := range pop.op.Parameters {
				// Resolve parameter-level $ref (#/parameters/Name).
				if raw.Ref != "" {
					if resolved, ok := paramDefs[refName(raw.Ref)]; ok {
						raw = resolved
					} else {
						fmt.Fprintf(os.Stderr, "warning: cannot resolve parameter $ref %q; skipping\n", raw.Ref)
						continue
					}
				}
				if raw.In == "body" {
					// Swagger 2.0 body param: inline or $ref schema (#/definitions).
					if raw.Schema != nil {
						od.BodyParams = append(od.BodyParams, bodyParamsFromSchema(*raw.Schema, definitions)...)
						if len(od.BodyParams) > 0 {
							cli.HasBodyParams = true
						}
					}
					continue
				}
				typ := firstNonEmpty(raw.Type, "string")
				itemType := ""
				if raw.Items != nil {
					itemType = raw.Items.Type
				}
				pd := makeParamData(raw.Name, raw.Description, raw.Required, typ, itemType)
				switch raw.In {
				case "path":
					pd.PathPlaceholder = "{" + raw.Name + "}"
					od.PathParams = append(od.PathParams, pd)
					cli.HasPathParams = true
				case "query":
					od.QueryParams = append(od.QueryParams, pd)
				case "header":
					od.HeaderParams = append(od.HeaderParams, pd)
				}
			}
			cmd.Operations = append(cmd.Operations, od)
		}
		if len(cmd.Operations) > 0 {
			cli.Commands = append(cli.Commands, cmd)
		}
	}
}

// versionFromBytes peeks at the spec bytes to detect "swagger" vs "openapi".
func versionFromBytes(data []byte) string {
	// Fast heuristic: look for top-level swagger or openapi key.
	var peek struct {
		Swagger string `yaml:"swagger" json:"swagger"`
		OpenAPI string `yaml:"openapi" json:"openapi"`
	}
	// Try YAML
	if yaml.Unmarshal(data, &peek) == nil {
		if strings.HasPrefix(peek.Swagger, "2") {
			return "swagger2"
		}
		if peek.OpenAPI != "" {
			return "openapi3"
		}
	}
	// Try JSON
	if json.Unmarshal(data, &peek) == nil {
		if strings.HasPrefix(peek.Swagger, "2") {
			return "swagger2"
		}
		if peek.OpenAPI != "" {
			return "openapi3"
		}
	}
	// Fallback: look for raw string markers
	s := string(data)
	if regexp.MustCompile(`(?m)^swagger:\s*["']?2`).MatchString(s) {
		return "swagger2"
	}
	return "openapi3"
}
