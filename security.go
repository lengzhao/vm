package vm

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// SecurityReviewer 安全审查模块接口
type SecurityReviewer interface {
	// Review 对合约源代码进行安全审查
	Review(sourceCode string) error

	// IsKeywordAllowed 检查关键字是否被允许
	IsKeywordAllowed(keyword string) bool

	// IsImportAllowed 检查导入是否被允许
	IsImportAllowed(importPath string) bool
}

// SecurityReviewerImpl 安全审查模块实现
type SecurityReviewerImpl struct {
	allowedKeywords map[string]bool
	allowedImports  map[string]bool
}

// NewSecurityReviewer 创建新的安全审查模块实例
func NewSecurityReviewer() SecurityReviewer {
	reviewer := &SecurityReviewerImpl{
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
func (s *SecurityReviewerImpl) Review(sourceCode string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.AllErrors)
	if err != nil {
		return err
	}

	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")
		if !s.IsImportAllowed(importPath) {
			return &SecurityError{
				Message:    "不允许的导入: " + importPath,
				ErrorType:  ImportNotAllowed,
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
			return &SecurityError{
				Message:   fmt.Sprintf("不允许包级可变全局变量: %s", strings.Join(names, ", ")),
				ErrorType: GlobalVarNotAllowed,
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
			reviewErr = &SecurityError{
				Message:   "不允许的关键字: go",
				ErrorType: KeywordNotAllowed,
				Keyword:   "go",
			}
			return false
		case *ast.SelectStmt:
			reviewErr = &SecurityError{
				Message:   "不允许的关键字: select",
				ErrorType: KeywordNotAllowed,
				Keyword:   "select",
			}
			return false
		case *ast.ChanType:
			reviewErr = &SecurityError{
				Message:   "不允许的关键字: chan",
				ErrorType: KeywordNotAllowed,
				Keyword:   "chan",
			}
			return false
		case *ast.MapType:
			reviewErr = &SecurityError{
				Message:   "不允许的关键字: map",
				ErrorType: KeywordNotAllowed,
				Keyword:   "map",
			}
			return false
		case *ast.BranchStmt:
			if x.Tok == token.GOTO {
				reviewErr = &SecurityError{
					Message:   "不允许的关键字: goto",
					ErrorType: KeywordNotAllowed,
					Keyword:   "goto",
				}
				return false
			}
		case *ast.CallExpr:
			if ident, ok := x.Fun.(*ast.Ident); ok && ident.Name == "cap" {
				reviewErr = &SecurityError{
					Message:   "不允许的关键字: cap",
					ErrorType: KeywordNotAllowed,
					Keyword:   "cap",
				}
				return false
			}
		case *ast.SelectorExpr:
			if ident, ok := x.X.(*ast.Ident); ok && ident.Name == "unsafe" {
				reviewErr = &SecurityError{
					Message:   "不允许的关键字: unsafe",
					ErrorType: KeywordNotAllowed,
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
func (s *SecurityReviewerImpl) IsKeywordAllowed(keyword string) bool {
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
func (s *SecurityReviewerImpl) IsImportAllowed(importPath string) bool {
	return s.allowedImports[importPath]
}

// SecurityError 安全审查错误
type SecurityError struct {
	Message    string
	ErrorType  SecurityErrorType
	ImportPath string
	Keyword    string
}

// SecurityErrorType 安全错误类型
type SecurityErrorType int

const (
	ImportNotAllowed SecurityErrorType = iota
	KeywordNotAllowed
	GlobalVarNotAllowed
)

func (e *SecurityError) Error() string {
	return e.Message
}
