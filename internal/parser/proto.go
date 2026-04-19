package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// ParseProto parses a .proto file and returns a CLIData for a gRPC CLI.
// Generated CLIs shell out to grpcurl for the actual RPC calls.
//
// Supports: service definitions, rpc methods, message fields.
// Does not support: imports, nested messages, oneofs, maps (treated as string).
func ParseProto(data []byte, source string) (*CLIData, error) {
	content := string(data)

	pkg := extractProtoPackage(content)
	servicePath, serverAddr := extractProtoOption(content)

	services := extractServices(content)
	if len(services) == 0 {
		return nil, fmt.Errorf("no service definitions found in %s", source)
	}

	// Derive CLI name from package or first service
	cliName := pkg
	if cliName == "" {
		cliName = ToCommandName(services[0].name)
	}

	cli := &CLIData{
		Name:        cliName,
		Description: fmt.Sprintf("gRPC CLI for %s", cliName),
		BaseURL:     serverAddr,
		Transport:   TransportGRPC,
		AuthEnvVar:  strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(cliName, "_")) + "__TOKEN",
	}

	// Extract all message definitions for field lookup
	messages := extractMessages(content)

	for _, svc := range services {
		cmd := CommandData{
			Name:        ToCommandName(svc.name),
			GoName:      toPascalCase(svc.name),
			Description: svc.name,
		}

		for _, rpc := range svc.rpcs {
			inputFields := messages[rpc.inputType]
			var params []ParamData
			for _, f := range inputFields {
				params = append(params, protoFieldToParam(f))
			}

			od := OperationData{
				Name:        ToCommandName(rpc.name),
				GoName:      toPascalCase(rpc.name),
				Description: rpc.name,
				GRPCService: svc.name,
				GRPCMethod:  rpc.name,
				InputParams: params,
			}
			if servicePath != "" {
				od.GRPCService = servicePath + "." + svc.name
			}
			cmd.Operations = append(cmd.Operations, od)
		}

		if len(cmd.Operations) > 0 {
			cli.Commands = append(cli.Commands, cmd)
		}
	}

	return cli, nil
}

// --- proto AST structs ---

type protoService struct {
	name string
	rpcs []protoRPC
}

type protoRPC struct {
	name       string
	inputType  string
	outputType string
}

type protoField struct {
	name     string
	typeName string // string, int32, int64, float, double, bool, bytes, or message name
	repeated bool
}

// --- regex-based parser ---

var (
	rePackage  = regexp.MustCompile(`(?m)^package\s+([\w.]+)\s*;`)
	reOption   = regexp.MustCompile(`(?m)option\s+java_package\s*=\s*"([\w.]+)"`)
	reGoPackage = regexp.MustCompile(`(?m)option\s+go_package\s*=\s*"([^"]+)"`)
	reService  = regexp.MustCompile(`(?s)service\s+(\w+)\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	reRPC      = regexp.MustCompile(`rpc\s+(\w+)\s*\(\s*(\w+)\s*\)\s*returns\s*\(\s*(\w+)\s*\)`)
	reMessage  = regexp.MustCompile(`(?s)message\s+(\w+)\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	reField    = regexp.MustCompile(`(?m)^\s*(?:(repeated)\s+)?(\w+)\s+(\w+)\s*=\s*\d+\s*;`)
	reServerOption  = regexp.MustCompile(`(?m)//\s*@server:\s*(\S+)`)
	reServiceOption = regexp.MustCompile(`(?m)//\s*@service:\s*(\S+)`)
)

func extractProtoPackage(content string) string {
	// Prefer go_package last segment
	if m := reGoPackage.FindStringSubmatch(content); m != nil {
		parts := strings.Split(m[1], "/")
		return ToCommandName(parts[len(parts)-1])
	}
	if m := rePackage.FindStringSubmatch(content); m != nil {
		parts := strings.Split(m[1], ".")
		return ToCommandName(parts[len(parts)-1])
	}
	return ""
}

func extractProtoOption(content string) (servicePath, serverAddr string) {
	// Look for a comment like: // @server: localhost:50051
	if m := reServerOption.FindStringSubmatch(content); m != nil {
		serverAddr = m[1]
	} else {
		serverAddr = "localhost:50051"
	}
	// Look for a comment like: // @service: com.example.v1
	if m := reServiceOption.FindStringSubmatch(content); m != nil {
		servicePath = m[1]
	} else {
		// Fallback: derive from proto package declaration (full package string)
		if m := rePackage.FindStringSubmatch(content); m != nil {
			servicePath = m[1]
		}
	}
	return
}

func extractServices(content string) []protoService {
	var services []protoService
	for _, m := range reService.FindAllStringSubmatch(content, -1) {
		svc := protoService{name: m[1]}
		for _, rm := range reRPC.FindAllStringSubmatch(m[2], -1) {
			svc.rpcs = append(svc.rpcs, protoRPC{
				name:       rm[1],
				inputType:  rm[2],
				outputType: rm[3],
			})
		}
		if len(svc.rpcs) > 0 {
			services = append(services, svc)
		}
	}
	return services
}

func extractMessages(content string) map[string][]protoField {
	msgs := map[string][]protoField{}
	for _, m := range reMessage.FindAllStringSubmatch(content, -1) {
		name := m[1]
		var fields []protoField
		for _, fm := range reField.FindAllStringSubmatch(m[2], -1) {
			fields = append(fields, protoField{
				repeated: fm[1] == "repeated",
				typeName: fm[2],
				name:     fm[3],
			})
		}
		msgs[name] = fields
	}
	return msgs
}

func protoFieldToParam(f protoField) ParamData {
	pd := ParamData{
		Name:      f.name,
		GoVarName: toPascalCase(f.name),
		FlagName:  toKebabCase(f.name),
	}

	switch f.typeName {
	case "int32", "int64", "uint32", "uint64", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64":
		pd.GoType = "int"
		pd.FlagFunc = "IntVar"
		pd.DefaultLiteral = "0"
		pd.DefaultCmp = "!= 0"
	case "float", "double":
		pd.GoType = "float64"
		pd.FlagFunc = "Float64Var"
		pd.DefaultLiteral = "0.0"
		pd.DefaultCmp = "!= 0.0"
	default:
		// string, bytes, bool, message types — treat as string
		pd.GoType = "string"
		pd.FlagFunc = "StringVar"
		pd.DefaultLiteral = `""`
		pd.DefaultCmp = `!= ""`
	}

	if f.repeated {
		// repeated fields as comma-separated string
		pd.GoType = "string"
		pd.FlagFunc = "StringVar"
		pd.DefaultLiteral = `""`
		pd.DefaultCmp = `!= ""`
		pd.Description = "(comma-separated)"
	}

	return pd
}
