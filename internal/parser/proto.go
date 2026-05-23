package parser

import (
	"fmt"
	"os"
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

	safeName := strings.ToUpper(regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(cliName, "_"))
	cli := &CLIData{
		Name:          cliName,
		Description:   fmt.Sprintf("gRPC CLI for %s", cliName),
		BaseURL:       serverAddr,
		BaseURLEnvVar: safeName + "__SERVER_ADDR",
		Transport:     TransportGRPC,
		AuthEnvVar:    safeName + "__TOKEN",
	}

	// @noauth annotation: disable auth requirement for this service.
	if reNoAuth.MatchString(content) {
		cli.AuthEnvVar = ""
	}

	// Extract all message definitions for field lookup
	messages := extractMessages(content)

	for _, svc := range services {
		cmd := CommandData{
			Name:        ToCommandName(svc.name),
			GoName:      toPascalCase(svc.name),
			Description: svc.name,
		}

		seenNames := map[string]int{} // dedup within this service
		for i, rpc := range svc.rpcs {
			if rpc.streaming {
				fmt.Fprintf(os.Stderr, "warning: skipping streaming RPC %s.%s (streaming not supported)\n", svc.name, rpc.name)
				continue
			}
			inputFields := messages[rpc.inputType]
			var params []ParamData
			for _, f := range inputFields {
				params = append(params, protoFieldToParam(f))
			}

			opName := ToCommandName(rpc.name)
			if opName == "" {
				// RPC name normalizes to empty; use stable index-based fallback.
				opName = fmt.Sprintf("rpc-%d", i)
			} else if count, exists := seenNames[opName]; exists {
				// Name collides after normalization; disambiguate within service.
				seenNames[opName] = count + 1
				opName = fmt.Sprintf("%s-%d", opName, count+1)
			} else {
				seenNames[opName] = 1
			}

			od := OperationData{
				Name:        opName,
				GoName:      toPascalCase(opName),
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

	if len(cli.Commands) == 0 {
		return nil, fmt.Errorf("no non-streaming operations found in %s", source)
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
	streaming  bool // true if either side uses the stream keyword
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
	reRPC      = regexp.MustCompile(`rpc\s+(\w+)\s*\(\s*(stream\s+)?(\w+)\s*\)\s*returns\s*\(\s*(stream\s+)?(\w+)\s*\)`)
	reMessage  = regexp.MustCompile(`(?s)message\s+(\w+)\s*\{([^}]*(?:\{[^}]*\}[^}]*)*)\}`)
	reField    = regexp.MustCompile(`(?m)^\s*(?:(repeated)\s+)?(\w+)\s+(\w+)\s*=\s*\d+\s*;`)
	reServerOption  = regexp.MustCompile(`(?m)//\s*@server:\s*(\S+)`)
	reServiceOption = regexp.MustCompile(`(?m)//\s*@service:\s*(\S+)`)
	reNoAuth        = regexp.MustCompile(`(?m)//\s*@noauth\b`)
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
		fmt.Fprintf(os.Stderr, "warning: no // @server: annotation found; defaulting to localhost:50051\n")
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
			// rm[1]=name, rm[2]=stream? (input), rm[3]=inputType,
			// rm[4]=stream? (output), rm[5]=outputType
			svc.rpcs = append(svc.rpcs, protoRPC{
				name:       rm[1],
				inputType:  rm[3],
				outputType: rm[5],
				streaming:  rm[2] != "" || rm[4] != "",
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
	case "bool":
		pd.GoType = "bool"
		pd.FlagFunc = "BoolVar"
		pd.DefaultLiteral = "false"
		pd.DefaultCmp = "!= false"
	default:
		// string, bytes, message types — treat as string
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
