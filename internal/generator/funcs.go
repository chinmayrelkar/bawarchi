package generator

import (
	"strings"
	"text/template"
)

// tmplFuncs are shared across all code generation templates.
var tmplFuncs = template.FuncMap{
	// safeStr makes a string safe to embed inside a Go double-quoted string literal:
	// strips newlines, tabs, and escapes backslashes and double-quotes.
	"safeStr": func(s string) string {
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		s = strings.ReplaceAll(s, "\n", " ")
		s = strings.ReplaceAll(s, "\r", "")
		s = strings.ReplaceAll(s, "\t", " ")
		return strings.TrimSpace(s)
	},
}
