package security

import (
	"fmt"
	"github.com/lengzhao/vm/core"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// Reviewer 安全审查模块实现
type Reviewer struct {
	allowedKeywords map[string]bool
	allowedImports  map[string]bool
}

// NewSecurityReviewer 创建新的安全审查模块实例
func New() *Reviewer {
	reviewer := &Reviewer{
		allowedKeywords: make(map[string]bool),
		allowedImports:  make(map[string]bool),
	}

	allowedKeywords := []string{
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128",
		"string", "bool", "byte", "rune",
		"const", "iota", "var", "type", "struct", "interface", "func",
		"if", "else", "for", "switch", "case", "default", "break", "continue", "fallthrough", "return",
		"package", "import", "nil", "true", "false",
		"len", "new", "make", "append", "copy", "delete",
	}

	for _, keyword := range allowedKeywords {
		reviewer.allowedKeywords[keyword] = true
	}

	allowedImports := []string{
		"fmt",
		"strconv",
		"math",
		"time",
		"errors",
		"github.com/lengzhao/vm/contractapi",
	}

	for _, imp := range allowedImports {
		reviewer.allowedImports[imp] = true
	}

	return reviewer
}

// Review 对合约源代码进行安全审查
func (s *Reviewer) Review(sourceCode string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.AllErrors)
	if err != nil {
		return err
	}

	if err := ReviewRuntimeBoundary(file); err != nil {
		return err
	}

	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		if !s.IsImportAllowed(importPath) {
			return &core.SecurityError{
				Message:    "不允许的导入: " + importPath,
				ErrorType:  core.ImportNotAllowed,
				ImportPath: importPath,
			}
		}
	}

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			names := make([]string, 0, len(valueSpec.Names))
			for _, name := range valueSpec.Names {
				names = append(names, name.Name)
			}
			return &core.SecurityError{
				Message:   fmt.Sprintf("不允许包级可变全局变量: %s", strings.Join(names, ", ")),
				ErrorType: core.GlobalVarNotAllowed,
				Keyword:   "var",
			}
		}
	}

	var reviewErr error
	ast.Inspect(file, func(n ast.Node) bool {
		if reviewErr != nil {
			return false
		}

		switch x := n.(type) {
		case *ast.GoStmt:
			reviewErr = &core.SecurityError{
				Message:   "不允许的关键字: go",
				ErrorType: core.KeywordNotAllowed,
				Keyword:   "go",
			}
			return false
		case *ast.SelectStmt:
			reviewErr = &core.SecurityError{
				Message:   "不允许的关键字: select",
				ErrorType: core.KeywordNotAllowed,
				Keyword:   "select",
			}
			return false
		case *ast.ChanType:
			reviewErr = &core.SecurityError{
				Message:   "不允许的关键字: chan",
				ErrorType: core.KeywordNotAllowed,
				Keyword:   "chan",
			}
			return false
		case *ast.MapType:
			reviewErr = &core.SecurityError{
				Message:   "不允许的关键字: map",
				ErrorType: core.KeywordNotAllowed,
				Keyword:   "map",
			}
			return false
		case *ast.BranchStmt:
			if x.Tok == token.GOTO {
				reviewErr = &core.SecurityError{
					Message:   "不允许的关键字: goto",
					ErrorType: core.KeywordNotAllowed,
					Keyword:   "goto",
				}
				return false
			}
		case *ast.CallExpr:
			if ident, ok := x.Fun.(*ast.Ident); ok && ident.Name == "cap" {
				reviewErr = &core.SecurityError{
					Message:   "不允许的关键字: cap",
					ErrorType: core.KeywordNotAllowed,
					Keyword:   "cap",
				}
				return false
			}
		case *ast.SelectorExpr:
			if ident, ok := x.X.(*ast.Ident); ok && ident.Name == "unsafe" {
				reviewErr = &core.SecurityError{
					Message:   "不允许的关键字: unsafe",
					ErrorType: core.KeywordNotAllowed,
					Keyword:   "unsafe",
				}
				return false
			}
		}
		return true
	})

	return reviewErr
}

// IsKeywordAllowed 检查关键字是否被允许
func (s *Reviewer) IsKeywordAllowed(keyword string) bool {
	dangerousKeywords := map[string]bool{
		"unsafe": true,
		"go":     true,
		"select": true,
		"chan":   true,
		"goto":   true,
		"map":    true,
		"cap":    true,
	}

	if dangerousKeywords[keyword] {
		return false
	}

	if s.allowedKeywords[keyword] {
		return true
	}

	return true
}

// IsImportAllowed 检查导入是否被允许
func (s *Reviewer) IsImportAllowed(importPath string) bool {
	return s.allowedImports[importPath]
}

// ReviewRuntimeBoundary keeps compiler-owned controls inaccessible to contract source.
// It also runs when optional language security checks are disabled.
func ReviewRuntimeBoundary(file *ast.File) error {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return err
		}
		if path != "github.com/lengzhao/vm/contractapi" {
			continue
		}
		name := "contractapi"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			return fmt.Errorf("contractapi dot import is not allowed")
		}
		aliases[name] = true
	}
	allowed := map[string]bool{"BlockHeight": true, "BlockTime": true, "Sender": true, "ContractAddress": true, "Log": true, "DefaultObject": true, "CreateObject": true, "OpenObject": true, "Call": true}
	var boundaryErr error
	ast.Inspect(file, func(n ast.Node) bool {
		if boundaryErr != nil {
			return false
		}
		switch x := n.(type) {
		case *ast.Ident:
			if strings.HasPrefix(x.Name, "__vm") || x.Name == "recover" {
				boundaryErr = fmt.Errorf("reserved runtime identifier: %s", x.Name)
			}
		case *ast.FuncDecl:
			if x.Recv == nil && x.Name.Name == "init" {
				boundaryErr = fmt.Errorf("contract init functions are not allowed")
			}
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && id.Obj == nil && aliases[id.Name] && !allowed[x.Sel.Name] {
				boundaryErr = fmt.Errorf("contractapi runtime symbol is not allowed: %s", x.Sel.Name)
			}
		}
		return boundaryErr == nil
	})
	return boundaryErr
}
