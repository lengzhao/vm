package gas

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
)

type Injector struct{}

func (*Injector) InjectGas(sourceCode string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.ParseComments)
	if err != nil {
		return "", err
	}

	amount := uint64(1)

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Body == nil || x.Name.Name == "main" {
				return true
			}
			x.Body.List = append([]ast.Stmt{consumeGasStmt(amount)}, x.Body.List...)
		case *ast.FuncLit:
			if x.Body != nil {
				x.Body.List = append([]ast.Stmt{consumeGasStmt(amount)}, x.Body.List...)
			}
		case *ast.ForStmt:
			if x.Body == nil {
				return true
			}
			x.Body.List = append([]ast.Stmt{consumeGasStmt(amount)}, x.Body.List...)
		case *ast.RangeStmt:
			if x.Body == nil {
				return true
			}
			x.Body.List = append([]ast.Stmt{consumeGasStmt(amount)}, x.Body.List...)
		}
		return true
	})

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func consumeGasStmt(amount uint64) ast.Stmt {
	return &ast.ExprStmt{
		X: &ast.CallExpr{
			Fun: ast.NewIdent("__vmConsumeGas"),
			Args: []ast.Expr{
				&ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", amount)},
			},
		},
	}
}
