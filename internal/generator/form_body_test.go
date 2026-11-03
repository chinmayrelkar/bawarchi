package generator

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chinmayrelkar/bawarchi/internal/parser"
)

// formRESTData returns a CLIData with a POST operation whose body is
// form-urlencoded, mirroring what the parser produces for Zulip-style specs.
func formRESTData() *parser.CLIData {
	str := func(name, goName string, required bool) parser.ParamData {
		return parser.ParamData{Name: name, GoVarName: goName, FlagName: name, GoType: "string", FlagFunc: "StringVar", DefaultLiteral: `""`, DefaultCmp: `!= ""`, ZeroCmp: `== ""`, Required: required}
	}
	ids := str("user_ids", "UserIds", false)
	ids.FlagName, ids.IsArray, ids.ElemType, ids.FormJSON = "user-ids", true, "int", true
	tags := str("tags", "Tags", false)
	tags.IsArray, tags.ElemType = true, "string"
	getIDs := ids
	getIDs.Name, getIDs.GoVarName, getIDs.FlagName = "group_ids", "GroupIds", "group-ids"
	d := minimalRESTData()
	d.HasArrayParams = true
	d.Commands[0].Operations = append(d.Commands[0].Operations, parser.OperationData{
		Name:     "send",
		GoName:   "Send",
		Method:   "POST",
		Path:     "/messages",
		FormBody: true,
		BodyParams: []parser.ParamData{
			str("type", "Type", true),
			str("to", "To", true),
			str("content", "Content", true),
			{Name: "read_by_sender", GoVarName: "ReadBySender", FlagName: "read-by-sender", GoType: "bool", FlagFunc: "BoolVar", DefaultLiteral: "false", DefaultCmp: "!= false", ZeroCmp: "== false"},
			ids,
			tags,
		},
	}, parser.OperationData{
		// Some specs (Zulip) declare a form requestBody on GET; it must be
		// sent as a query string, since GET carries no body.
		Name:       "groups",
		GoName:     "Groups",
		Method:     "GET",
		Path:       "/groups",
		FormBody:   true,
		BodyParams: []parser.ParamData{getIDs},
	})
	return d
}

func TestGenerateREST_FormBody_EncodesForm(t *testing.T) {
	src := genFor(t, formRESTData())
	for _, want := range []string{
		`form := url.Values{}`,
		`hdrs["Content-Type"] = "application/x-www-form-urlencoded"`,
		`form.Set("to", fmt.Sprintf("%v", pTo))`,
		`json.Marshal(toIntList(pUserIds))`,
		`form.Add("tags", v)`,
		`doRequest("POST", u.String(), strings.NewReader(form.Encode()), hdrs)`,
		`error: --content is required`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated form op missing %q", want)
		}
	}
	if strings.Contains(src, "reqBody := map[string]interface{}{}") {
		t.Error("form op must not build a JSON reqBody")
	}
	if strings.Contains(src, `"bytes"`) {
		t.Error(`form-only CLI must not import "bytes" (unused import compile error)`)
	}
	if !strings.Contains(src, "form := q") || !strings.Contains(src, `doRequest("GET", u.String(), nil, nil)`+"\n}\n\n// splitList") {
		t.Error("GET form op must put its fields in the query string and send no body")
	}
	// GET op is unaffected.
	if !strings.Contains(src, `doRequest("GET", u.String(), nil, nil)`) {
		t.Error("ops without a body must still call doRequest(..., nil, nil)")
	}
}

// TestGenerateREST_FormBody_EndToEnd compiles the generated CLI and runs it
// against a local server to check the wire format: Content-Type, scalar
// values, JSON-encoded arrays, and repeated keys for plain arrays.
func TestGenerateREST_FormBody_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}

	var gotCT, gotQuery, gotBody string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotQuery = r.URL.RawQuery
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		gotForm, _ = url.ParseQuery(gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"result":"success"}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := Generate(formRESTData(), dir); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	bin := filepath.Join(dir, "cli")
	build := exec.Command(goBin, "build", "-o", bin, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("generated CLI does not compile: %v\n%s", err, out)
	}

	run := exec.Command(bin, "pets", "send",
		"-type", "direct", "-to", "[9,10]", "-content", "**hi** & bye",
		"-read-by-sender", "-user-ids", "1, 2", "-tags", "a,b")
	run.Env = append(os.Environ(), "TESTREST__BASE_URL="+srv.URL, "TESTREST_API_KEY=k")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("running generated CLI: %v\n%s", err, out)
	}

	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", gotCT)
	}
	want := map[string][]string{
		"type":           {"direct"},
		"to":             {"[9,10]"},
		"content":        {"**hi** & bye"},
		"read_by_sender": {"true"},
		"user_ids":       {"[1,2]"},
		"tags":           {"a", "b"},
	}
	for k, v := range want {
		if strings.Join(gotForm[k], "|") != strings.Join(v, "|") {
			t.Errorf("form[%q] = %q, want %q", k, gotForm[k], v)
		}
	}
	if gotQuery != "" {
		t.Errorf("POST form op must not use the query string, got %q", gotQuery)
	}

	run = exec.Command(bin, "pets", "groups", "-group-ids", "3,4")
	run.Env = append(os.Environ(), "TESTREST__BASE_URL="+srv.URL, "TESTREST_API_KEY=k")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("running generated CLI (GET): %v\n%s", err, out)
	}
	if q, _ := url.ParseQuery(gotQuery); q.Get("group_ids") != "[3,4]" {
		t.Errorf("GET form field must be in the query string, got query %q", gotQuery)
	}
	if gotBody != "" || gotCT != "" {
		t.Errorf("GET form op must send no body or Content-Type, got body %q Content-Type %q", gotBody, gotCT)
	}
}
