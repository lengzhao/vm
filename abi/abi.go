package abi

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// Import represents an import statement in the contract
type Import struct {
	Path string `json:"path,omitempty"` // 导入路径
	Name string `json:"name,omitempty"` // 别名（如果有）
}

// ABI represents the Application Binary Interface of a contract
type ABI struct {
	Version     int        `json:"version"`
	PackageName string     `json:"package_name,omitempty"`
	Imports     []Import   `json:"imports,omitempty"`
	Functions   []Function `json:"functions,omitempty"`
	Events      []Event    `json:"events,omitempty"`
}

// Function represents a function in the contract
type Function struct {
	Generic bool        `json:"generic,omitempty"`
	Name    string      `json:"name,omitempty"`
	Inputs  []Parameter `json:"inputs,omitempty"`
	Outputs []Parameter `json:"outputs,omitempty"`
}

// Event represents a contract event
type Event struct {
	Name       string      `json:"name,omitempty"`
	Parameters []Parameter `json:"parameters,omitempty"`
}

// Parameter represents a function parameter or event field
type Parameter struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// ExtractABI extracts the ABI information from contract code
func ExtractABI(code []byte) (*ABI, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", code, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("failed to parse contract: %w", err)
	}

	abi := &ABI{
		Version:     SchemaVersion,
		PackageName: file.Name.Name,
		Imports:     make([]Import, 0),
		Functions:   make([]Function, 0),
		Events:      make([]Event, 0),
	}

	// 提取导入信息
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		importInfo := Import{Path: path}
		if imp.Name != nil {
			importInfo.Name = imp.Name.Name
		}
		abi.Imports = append(abi.Imports, importInfo)
	}

	// Extract package-level functions and events
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			// Skip methods (functions with receivers)
			if funcDecl.Recv != nil {
				continue
			}

			// Only include exported functions
			if !funcDecl.Name.IsExported() {
				continue
			}

			function := Function{
				Name:    funcDecl.Name.Name,
				Generic: funcDecl.Type.TypeParams != nil,
			}

			// Extract input parameters
			if funcDecl.Type.Params != nil {
				function.Inputs = extractParameters(funcDecl.Type.Params)
			}

			// Extract output parameters
			if funcDecl.Type.Results != nil {
				function.Outputs = extractParameters(funcDecl.Type.Results)
			}

			// Extract events from function body
			events := extractEventsFromFunction(funcDecl, eventImports(file))
			abi.Events = append(abi.Events, events...)

			abi.Functions = append(abi.Functions, function)
		}
	}

	return abi, nil
}

// extractEventsFromFunction extracts events from a function's body
func eventImports(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		if path != "github.com/lengzhao/vm/contractapi" {
			continue
		}
		name := "contractapi"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name != "." && name != "_" {
			aliases[name] = true
		}
	}
	return aliases
}

func staticString(expr ast.Expr) (string, bool) {
	if id, ok := expr.(*ast.Ident); ok && id.Obj != nil && id.Obj.Kind == ast.Con {
		if spec, ok := id.Obj.Decl.(*ast.ValueSpec); ok {
			for i, n := range spec.Names {
				if n.Name == id.Name && i < len(spec.Values) {
					expr = spec.Values[i]
					break
				}
			}
		}
	}
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func extractEventsFromFunction(fn *ast.FuncDecl, aliases map[string]bool) []Event {
	events := []Event{}
	if fn.Body == nil {
		return events
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Log" {
			return true
		}
		owner, ok := selector.X.(*ast.Ident)
		if !ok || owner.Obj != nil || !aliases[owner.Name] {
			return true
		}
		name, known := staticString(call.Args[0])
		if !known {
			return true
		}
		event := Event{Name: name, Parameters: []Parameter{}}
		for i := 1; i+1 < len(call.Args); i += 2 {
			key, known := staticString(call.Args[i])
			if known {
				event.Parameters = append(event.Parameters, Parameter{Name: key, Type: eventValueType(call.Args[i+1], aliases, 0)})
			}
		}
		events = append(events, event)
		return true
	})
	return events
}

func eventValueType(expr ast.Expr, aliases map[string]bool, depth int) string {
	if depth > 8 {
		return "unknown"
	}
	switch x := expr.(type) {
	case *ast.BasicLit:
		return getTypeString(x)
	case *ast.UnaryExpr:
		return eventValueType(x.X, aliases, depth+1)
	case *ast.Ident:
		if x.Obj == nil {
			if x.Name == "true" || x.Name == "false" {
				return "bool"
			}
			return "unknown"
		}
		switch decl := x.Obj.Decl.(type) {
		case *ast.Field:
			return getTypeString(decl.Type)
		case *ast.ValueSpec:
			if decl.Type != nil {
				return getTypeString(decl.Type)
			}
			for i, n := range decl.Names {
				if n.Name == x.Name && i < len(decl.Values) {
					return eventValueType(decl.Values[i], aliases, depth+1)
				}
			}
		case *ast.AssignStmt:
			for i, n := range decl.Lhs {
				if id, ok := n.(*ast.Ident); ok && id.Name == x.Name && i < len(decl.Rhs) {
					return eventValueType(decl.Rhs[i], aliases, depth+1)
				}
			}
		}
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok && id.Obj == nil && IsScalarType(id.Name) {
			return id.Name
		}
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil && aliases[id.Name] {
				switch sel.Sel.Name {
				case "BlockHeight", "BlockTime":
					return "uint64"
				case "Sender", "ContractAddress":
					return "string"
				}
			}
		}
	}
	return "unknown"
}

// extractParameters extracts parameter information from a field list
func extractParameters(fieldList *ast.FieldList) []Parameter {
	if fieldList == nil {
		return nil
	}

	params := make([]Parameter, 0)
	for _, field := range fieldList.List {
		// Get parameter type
		typeStr := getTypeString(field.Type)

		// Handle named parameters
		if len(field.Names) > 0 {
			for _, name := range field.Names {
				params = append(params, Parameter{
					Name: name.Name,
					Type: typeStr,
				})
			}
		} else {
			// Handle unnamed parameters
			params = append(params, Parameter{
				Name: "",
				Type: typeStr,
			})
		}
	}

	return params
}

// getTypeString converts an ast.Expr to its string representation
func getTypeString(expr ast.Expr) string {
	if lit, ok := expr.(*ast.BasicLit); ok {
		switch lit.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "rune"
		}
	}
	var out bytes.Buffer
	if err := format.Node(&out, token.NewFileSet(), expr); err != nil {
		return "unknown"
	}
	return out.String()
}

// String returns a string representation of the ABI
func (abi *ABI) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Package: %s\n", abi.PackageName))

	sb.WriteString("\nImports:\n")
	for _, imp := range abi.Imports {
		if imp.Name != "" {
			sb.WriteString(fmt.Sprintf("  %s %s\n", imp.Name, imp.Path))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", imp.Path))
		}
	}

	sb.WriteString("\nFunctions:\n")
	for _, fn := range abi.Functions {
		sb.WriteString(fmt.Sprintf("  %s(", fn.Name))

		// Write input parameters
		for i, input := range fn.Inputs {
			if i > 0 {
				sb.WriteString(", ")
			}
			if input.Name != "" {
				sb.WriteString(fmt.Sprintf("%s %s", input.Name, input.Type))
			} else {
				sb.WriteString(input.Type)
			}
		}

		sb.WriteString(")")

		// Write output parameters
		if len(fn.Outputs) > 0 {
			sb.WriteString(" ")
			if len(fn.Outputs) == 1 {
				sb.WriteString(fn.Outputs[0].Type)
			} else {
				sb.WriteString("(")
				for i, output := range fn.Outputs {
					if i > 0 {
						sb.WriteString(", ")
					}
					if output.Name != "" {
						sb.WriteString(fmt.Sprintf("%s %s", output.Name, output.Type))
					} else {
						sb.WriteString(output.Type)
					}
				}
				sb.WriteString(")")
			}
		}

		sb.WriteString("\n")
	}

	sb.WriteString("\nEvents:\n")
	for _, event := range abi.Events {
		sb.WriteString(fmt.Sprintf("  %s(", event.Name))
		for i, param := range event.Parameters {
			if i > 0 {
				sb.WriteString(", ")
			}
			if param.Name != "" {
				sb.WriteString(fmt.Sprintf("%s %s", param.Name, param.Type))
			} else {
				sb.WriteString(param.Type)
			}
		}
		sb.WriteString(")\n")
	}

	return sb.String()
}
