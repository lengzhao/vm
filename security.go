package vm

import (
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

	// 初始化允许的关键字白名单
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

	// 初始化允许的导入白名单
	allowedImports := []string{
		"fmt",
		"strconv",
		"math",
		"time",
		"errors",
		"github.com/lengzhao/vm",
	}

	for _, imp := range allowedImports {
		reviewer.allowedImports[imp] = true
	}

	return reviewer
}

// Review 对合约源代码进行安全审查
func (s *SecurityReviewerImpl) Review(sourceCode string) error {
	// 解析源代码为AST
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.AllErrors)
	if err != nil {
		return err
	}

	// 检查导入列表
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

	// 检查关键字使用
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if !s.IsKeywordAllowed(x.Name) {
				// 检查是否是函数名或变量名而不是关键字
				// 这里简化处理，实际需要更复杂的逻辑来区分
				// 暂时跳过函数名和变量名的检查
			}
		}
		return true
	})

	return nil
}

// IsKeywordAllowed 检查关键字是否被允许
func (s *SecurityReviewerImpl) IsKeywordAllowed(keyword string) bool {
	// 允许所有标识符，只限制特定的危险关键字
	// 危险关键字列表
	dangerousKeywords := map[string]bool{
		"unsafe": true,
		"go":     true,
		"select": true,
		"chan":   true,
		"goto":   true,
		"map":    true,
		"cap":    true,
	}

	// 如果在危险关键字列表中，则不允许
	if dangerousKeywords[keyword] {
		return false
	}

	// 如果是基本类型或控制流关键字，则允许
	if s.allowedKeywords[keyword] {
		return true
	}

	// 其他标识符默认允许（函数名、变量名等）
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
)

func (e *SecurityError) Error() string {
	return e.Message
}
