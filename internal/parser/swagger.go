package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
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
}

// Swagger 2.0: type is on the parameter directly (not inside schema).
type swagger2Parameter struct {
	Name        string `yaml:"name" json:"name"`
	In          string `yaml:"in" json:"in"`
	Description string `yaml:"description" json:"description"`
	Required    bool   `yaml:"required" json:"required"`
	Type        string `yaml:"type" json:"type"`
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
	// except for how parameters encode their type — handled via getParamInfo.
	buildCommandsFromSwagger(cli, spec.Paths)

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

// buildCommandsFromSwagger re-parses paths with Swagger 2.0-aware parameter handling.
// Swagger 2.0 parameters have `type` directly on the parameter object; we re-unmarshal
// each operation's parameters using swagger2Parameter to get the type correctly.
func buildCommandsFromSwagger(cli *CLIData, rawPaths map[string]oaPathItem) {
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

	for path, item := range rawPaths {
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
			Name:        ToCommandName(tag),
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
			// Re-parse raw parameters as swagger2Parameter to pick up direct `type` field.
			for _, raw := range pop.op.Parameters {
				// raw.Name/In/Description/Required are populated from the shared oaParameter struct.
				// raw.Schema.Type is empty in Swagger 2.0; use raw.Type (via separate re-parse below).
				typ := swagger2ParamType(raw)
				pd := makeParamData(raw.Name, raw.Description, raw.Required, typ)
				switch raw.In {
				case "path":
					pd.PathPlaceholder = "{" + raw.Name + "}"
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

// swagger2ParamType extracts the type from a Swagger 2.0 parameter.
// The shared oaParameter struct has a Schema.Type field (OpenAPI 3.x) and no direct Type field.
// To get the Swagger 2.0 `type`, we re-encode/decode the parameter through yaml.
// This is intentional isolation: Swagger 2.0 type extraction doesn't touch OpenAPI 3.x code paths.
func swagger2ParamType(p oaParameter) string {
	// Re-marshal to YAML then unmarshal into swagger2Parameter to get the `type` field.
	raw, err := yaml.Marshal(p)
	if err != nil {
		return "string"
	}
	var sp swagger2Parameter
	if err := yaml.Unmarshal(raw, &sp); err != nil {
		return "string"
	}
	return firstNonEmpty(sp.Type, "string")
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
