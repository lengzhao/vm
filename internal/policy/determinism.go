package policy

import (
	"fmt"
	"github.com/lengzhao/vm/core"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// DeterministicPolicyVersion identifies the restricted source-language policy.
// Changing acceptance or execution semantics requires a new version.
const DeterministicPolicyVersion = "go-deterministic-v1"

func reviewDeterministic(source string, reviewer core.SecurityReviewer) error {
	if strconv.IntSize != 64 {
		return fmt.Errorf("%s requires a 64-bit target", DeterministicPolicyVersion)
	}
	if err := reviewer.Review(source); err != nil {
		return err
	}
	file, err := parser.ParseFile(token.NewFileSet(), "", source, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return err
	}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if strings.HasPrefix(comment.Text, "//go:") {
				return fmt.Errorf("%s: compiler directives are not allowed", DeterministicPolicyVersion)
			}
		}
	}
	aliases := map[string]string{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		switch path {
		case "strconv", "errors", "github.com/lengzhao/vm/contractapi":
		default:
			return fmt.Errorf("%s: import %q is not deterministic-profile API", DeterministicPolicyVersion, path)
		}
		name := path
		if path == "github.com/lengzhao/vm/contractapi" {
			name = "contractapi"
		}
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." || name == "_" {
			return fmt.Errorf("%s: dot and blank imports are not allowed", DeterministicPolicyVersion)
		}
		aliases[name] = path
	}
	// strconv.IntSize, platform-dependent conversions and floating point helpers
	// are intentionally excluded. All keys are referenced symbols, not just calls.
	strconvAPI := map[string]bool{
		"FormatBool": true, "FormatInt": true, "FormatUint": true,
		"ParseBool": true, "ParseInt": true, "ParseUint": true,
		"AppendBool": true, "AppendInt": true, "AppendUint": true,
		"Quote": true, "QuoteToASCII": true, "QuoteToGraphic": true,
		"QuoteRune": true, "QuoteRuneToASCII": true, "QuoteRuneToGraphic": true,
		"CanBackquote": true, "Unquote": true, "UnquoteChar": true,
		"IsPrint": true, "IsGraphic": true,
	}
	banned := map[string]bool{"float32": true, "float64": true, "complex64": true, "complex128": true, "uintptr": true, "complex": true, "real": true, "imag": true, "new": true, "panic": true, "print": true, "println": true}
	ast.Inspect(file, func(n ast.Node) bool {
		if err != nil {
			return false
		}
		reject := func(reason string) { err = fmt.Errorf("%s: %s", DeterministicPolicyVersion, reason) }
		switch x := n.(type) {
		case *ast.Ident:
			if x.Obj == nil && banned[x.Name] {
				reject("unsupported identifier " + x.Name)
			}
		case *ast.BasicLit:
			if x.Kind == token.FLOAT || x.Kind == token.IMAG {
				reject("floating-point and imaginary literals are not allowed")
			}
		case *ast.StarExpr:
			reject("pointer types and dereferences are not allowed")
		case *ast.UnaryExpr:
			if x.Op == token.AND {
				reject("address expressions are not allowed")
			}
		case *ast.SelectorExpr:
			id, ok := x.X.(*ast.Ident)
			if !ok || id.Obj != nil {
				break
			}
			switch aliases[id.Name] {
			case "strconv":
				if !strconvAPI[x.Sel.Name] {
					reject("strconv symbol is not allowed: " + x.Sel.Name)
				}
			case "errors":
				if x.Sel.Name != "New" {
					reject("errors symbol is not allowed: " + x.Sel.Name)
				}
			}
		}
		return err == nil
	})
	return err
}
