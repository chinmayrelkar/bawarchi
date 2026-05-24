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
// Structure (services, rpcs, messages, fields) is parsed by a depth-aware,
// brace-balanced tokenizer rather than line regexes, so nested messages, maps,
// oneofs, enums, field options, and block/line comments are handled correctly.
// Streaming RPCs and oneof fields are skipped with a warning.
func ParseProto(data []byte, source string) (*CLIData, error) {
	content := string(data)

	pkg := extractProtoPackage(content)
	servicePath, serverAddr := extractProtoOption(content)

	// Structural parse via the token parser.
	file := parseProtoFile(content)
	services := file.services
	messages := file.messages

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

	cli.finalize()
	return cli, nil
}

// --- proto AST structs ---

type protoFile struct {
	services []protoService
	messages map[string][]protoField
}

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
	typeName string // string, int32, int64, float, double, bool, bytes, map, or message/enum name
	repeated bool
}

// --- annotation / package extraction (operates on raw comments) ---

var (
	rePackage   = regexp.MustCompile(`(?m)^package\s+([\w.]+)\s*;`)
	reGoPackage = regexp.MustCompile(`(?m)option\s+go_package\s*=\s*"([^"]+)"`)

	reServerOption  = regexp.MustCompile(`(?m)//\s*@server:\s*(\S+)`)
	reServiceOption = regexp.MustCompile(`(?m)//\s*@service:\s*(\S+)`)
	reNoAuth        = regexp.MustCompile(`(?m)//\s*@noauth\b`)

	reIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
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

// --- depth-aware token parser ---

// parseProtoFile tokenizes the .proto source (comments stripped) and walks the
// top-level declarations, collecting services and message field sets.
func parseProtoFile(content string) protoFile {
	f := protoFile{messages: map[string][]protoField{}}
	toks := tokenizeProto(content)
	i := 0
	for i < len(toks) {
		switch toks[i] {
		case "message":
			i = parseMessage(toks, i+1, &f)
		case "service":
			i = parseService(toks, i+1, &f)
		case "enum":
			i = skipNamedBlock(toks, i+1)
		default:
			i = skipStatement(toks, i)
		}
	}
	return f
}

// parseService parses `Name { rpc ... }` starting at the name token (toks[i]).
// Returns the index just past the service's closing brace.
func parseService(toks []string, i int, f *protoFile) int {
	if i >= len(toks) {
		return len(toks)
	}
	name := toks[i]
	i++
	if i >= len(toks) || toks[i] != "{" {
		return i
	}
	end := matchBrace(toks, i)
	svc := protoService{name: name}
	j := i + 1
	for j < end-1 {
		if toks[j] == "rpc" {
			rpc, next := parseRPC(toks, j+1, end-1)
			if rpc != nil {
				svc.rpcs = append(svc.rpcs, *rpc)
			}
			if next <= j {
				next = j + 1
			}
			j = next
		} else {
			j++
		}
	}
	if len(svc.rpcs) > 0 {
		f.services = append(f.services, svc)
	}
	return end
}

// parseRPC parses `Name ( [stream] In ) returns ( [stream] Out ) (;|{...})`
// starting at the name token. Returns the parsed RPC and the next index.
func parseRPC(toks []string, i, limit int) (*protoRPC, int) {
	if i >= limit {
		return nil, limit
	}
	name := toks[i]
	i++
	if i >= limit || toks[i] != "(" {
		return nil, skipToSemicolon(toks, i, limit)
	}
	i++
	inStream := false
	if i < limit && toks[i] == "stream" {
		inStream = true
		i++
	}
	inType := ""
	if i < limit {
		inType = lastSegment(toks[i])
		i++
	}
	for i < limit && toks[i] != ")" {
		i++
	}
	i++ // past ')'
	if i < limit && toks[i] == "returns" {
		i++
	}
	if i < limit && toks[i] == "(" {
		i++
	}
	outStream := false
	if i < limit && toks[i] == "stream" {
		outStream = true
		i++
	}
	outType := ""
	if i < limit {
		outType = lastSegment(toks[i])
		i++
	}
	for i < limit && toks[i] != ")" {
		i++
	}
	i++ // past ')'
	// Optional method body { ... } or trailing ';'.
	if i < limit && toks[i] == "{" {
		i = matchBrace(toks, i)
	} else if i < limit && toks[i] == ";" {
		i++
	}
	return &protoRPC{name: name, inputType: inType, outputType: outType, streaming: inStream || outStream}, i
}

// parseMessage parses `Name { ... }` starting at the name token, recording the
// message's scalar/array/map fields (skipping nested messages, enums, oneofs,
// reserved/option statements). Returns the index past the closing brace.
func parseMessage(toks []string, i int, f *protoFile) int {
	if i >= len(toks) {
		return len(toks)
	}
	name := toks[i]
	i++
	if i >= len(toks) || toks[i] != "{" {
		return i
	}
	end := matchBrace(toks, i)
	var fields []protoField
	j := i + 1
	for j < end-1 {
		switch toks[j] {
		case "message":
			j = parseMessage(toks, j+1, f) // nested message: stored by simple name
		case "enum":
			j = skipNamedBlock(toks, j+1)
		case "oneof":
			oneofName := ""
			if j+1 < end {
				oneofName = toks[j+1]
			}
			fmt.Fprintf(os.Stderr, "warning: skipping oneof %q in message %q (oneof not supported)\n", oneofName, name)
			k := j + 1
			for k < end && toks[k] != "{" {
				k++
			}
			j = matchBrace(toks, k)
		case "reserved", "option", "extensions", "extend":
			j = skipToSemicolon(toks, j, end)
		case "map":
			fld, next := parseMapField(toks, j, end)
			if fld != nil {
				fields = append(fields, *fld)
			}
			j = next
		case ";":
			j++
		default:
			fld, next := parseField(toks, j, end)
			if fld != nil {
				fields = append(fields, *fld)
			}
			if next > j {
				j = next
			} else {
				j++
			}
		}
	}
	f.messages[name] = fields
	return end
}

// parseField parses `[repeated|optional|required] Type name = N [opts];`.
// Returns nil (and skips to the next ';') if the tokens are not a field.
func parseField(toks []string, j, end int) (*protoField, int) {
	repeated := false
	switch toks[j] {
	case "repeated":
		repeated = true
		j++
	case "optional", "required":
		j++
	}
	if j+2 >= end {
		return nil, skipToSemicolon(toks, j, end)
	}
	typeName := toks[j]
	name := toks[j+1]
	if toks[j+2] != "=" || !isIdent(typeName) || !isIdent(name) {
		return nil, skipToSemicolon(toks, j, end)
	}
	return &protoField{repeated: repeated, typeName: lastSegment(typeName), name: name}, skipToSemicolon(toks, j, end)
}

// parseMapField parses `map<K, V> name = N;` and represents it as a single
// string field (callers pass map values as raw JSON). Returns the next index.
func parseMapField(toks []string, j, end int) (*protoField, int) {
	k := j + 1 // past 'map'
	if k >= end || toks[k] != "<" {
		return nil, skipToSemicolon(toks, j, end)
	}
	depth := 0
	for k < end {
		if toks[k] == "<" {
			depth++
		} else if toks[k] == ">" {
			depth--
			if depth == 0 {
				k++
				break
			}
		}
		k++
	}
	if k >= end {
		return nil, end
	}
	name := toks[k]
	if k+1 < end && toks[k+1] == "=" && isIdent(name) {
		return &protoField{name: name, typeName: "map"}, skipToSemicolon(toks, j, end)
	}
	return nil, skipToSemicolon(toks, j, end)
}

// --- token helpers ---

// matchBrace returns the index just past the '}' matching the '{' at toks[open].
func matchBrace(toks []string, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		switch toks[i] {
		case "{":
			depth++
		case "}":
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(toks)
}

// skipNamedBlock skips `Name { ... }` starting at the name token.
func skipNamedBlock(toks []string, i int) int {
	for i < len(toks) && toks[i] != "{" {
		i++
	}
	return matchBrace(toks, i)
}

// skipToSemicolon returns the index just past the next ';' within [j,end),
// skipping any balanced brace blocks encountered first.
func skipToSemicolon(toks []string, j, end int) int {
	for j < end {
		switch toks[j] {
		case ";":
			return j + 1
		case "{":
			j = matchBrace(toks, j)
		default:
			j++
		}
	}
	return end
}

// skipStatement advances past a top-level statement (ends at ';' or a block).
func skipStatement(toks []string, i int) int {
	for i < len(toks) {
		switch toks[i] {
		case ";":
			return i + 1
		case "{":
			return matchBrace(toks, i)
		default:
			i++
		}
	}
	return len(toks)
}

// lastSegment strips a package/qualifier prefix: ".pkg.Msg" or "pkg.Msg" -> "Msg".
func lastSegment(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

func isIdent(s string) bool { return reIdent.MatchString(s) }

// tokenizeProto strips comments and splits the source into a flat token stream
// where structural punctuation characters are individual tokens.
func tokenizeProto(src string) []string {
	src = stripProtoComments(src)
	var toks []string
	i, n := 0, len(src)
	isPunct := func(c byte) bool {
		switch c {
		case '{', '}', '(', ')', '<', '>', '=', ';', ',', '[', ']':
			return true
		}
		return false
	}
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
	for i < n {
		c := src[i]
		switch {
		case isSpace(c):
			i++
		case c == '"' || c == '\'':
			quote := c
			j := i + 1
			for j < n && src[j] != quote {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			endIdx := j + 1
			if endIdx > n {
				endIdx = n
			}
			toks = append(toks, src[i:endIdx])
			i = endIdx
		case isPunct(c):
			toks = append(toks, string(c))
			i++
		default:
			j := i
			for j < n && !isPunct(src[j]) && !isSpace(src[j]) && src[j] != '"' && src[j] != '\'' {
				j++
			}
			toks = append(toks, src[i:j])
			i = j
		}
	}
	return toks
}

// stripProtoComments removes // line comments and /* block */ comments while
// preserving string-literal contents.
func stripProtoComments(src string) string {
	var b strings.Builder
	i, n := 0, len(src)
	for i < n {
		switch {
		case i+1 < n && src[i] == '/' && src[i+1] == '/':
			for i < n && src[i] != '\n' {
				i++
			}
		case i+1 < n && src[i] == '/' && src[i+1] == '*':
			i += 2
			for i+1 < n && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i += 2
		case src[i] == '"' || src[i] == '\'':
			q := src[i]
			b.WriteByte(src[i])
			i++
			for i < n && src[i] != q {
				if src[i] == '\\' && i+1 < n {
					b.WriteByte(src[i])
					i++
				}
				b.WriteByte(src[i])
				i++
			}
			if i < n {
				b.WriteByte(src[i])
				i++
			}
		default:
			b.WriteByte(src[i])
			i++
		}
	}
	return b.String()
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
		// string, bytes, map, message, enum types — treat as string
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
