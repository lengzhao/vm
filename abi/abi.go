package abi

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// Import represents an import statement in the contract
type Import struct {
	Path string `json:"path,omitempty"` // 导入路径
	Name string `json:"name,omitempty"` // 别名（如果有）
}

// ABI represents the Application Binary Interface of a contract
type ABI struct {
	PackageName string     `json:"package_name,omitempty"`
	Imports     []Import   `json:"imports,omitempty"`
	Functions   []Function `json:"functions,omitempty"`
	Events      []Event    `json:"events,omitempty"`
}

// Function represents a function in the contract
type Function struct {
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
		PackageName: file.Name.Name,
		Imports:     make([]Import, 0),
		Functions:   make([]Function, 0),
		Events:      make([]Event, 0),
	}

	// 提取导入信息
	for _, imp := range file.Imports {
		importInfo := Import{
			Path: strings.Trim(imp.Path.Value, "\""),
		}
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
				Name: funcDecl.Name.Name,
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
			events := extractEventsFromFunction(funcDecl)
			abi.Events = append(abi.Events, events...)

			abi.Functions = append(abi.Functions, function)
		}
	}

	return abi, nil
}

// extractEventsFromFunction extracts events from a function's body
func extractEventsFromFunction(funcDecl *ast.FuncDecl) []Event {
	events := make([]Event, 0)
	if funcDecl.Body == nil {
		return events
	}

	ast.Inspect(funcDecl.Body, func(node ast.Node) bool {
		callExpr, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if this is a ctx.Log call or similar event emission
		// 支持多种事件发射方式
		eventName := ""
		args := callExpr.Args

		// 检查函数调用
		switch fun := callExpr.Fun.(type) {
		case *ast.SelectorExpr:
			// 检查是否是 ctx.Log 或类似的调用
			if fun.Sel.Name == "Log" || fun.Sel.Name == "Emit" || fun.Sel.Name == "Event" ||
				fun.Sel.Name == "emit" || fun.Sel.Name == "log" || fun.Sel.Name == "event" {
				// 第一个参数作为事件名称
				if len(args) > 0 {
					if basicLit, ok := args[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
						eventName = strings.Trim(basicLit.Value, "\"")
						args = args[1:] // 移除事件名称参数
					} else if ident, ok := args[0].(*ast.Ident); ok {
						// 如果第一个参数是标识符，将其作为事件名称
						eventName = ident.Name
						args = args[1:] // 移除事件名称参数
					}
				}
			}
		case *ast.Ident:
			// 检查是否是直接的事件发射函数
			if fun.Name == "emit" || fun.Name == "log" || fun.Name == "event" ||
				fun.Name == "Emit" || fun.Name == "Log" || fun.Name == "Event" {
				// 第一个参数作为事件名称
				if len(args) > 0 {
					if basicLit, ok := args[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
						eventName = strings.Trim(basicLit.Value, "\"")
						args = args[1:] // 移除事件名称参数
					} else if ident, ok := args[0].(*ast.Ident); ok {
						// 如果第一个参数是标识符，将其作为事件名称
						eventName = ident.Name
						args = args[1:] // 移除事件名称参数
					}
				}
			}
		}

		// 如果找到了事件名称，创建事件对象
		if eventName != "" {
			event := Event{
				Name:       eventName,
				Parameters: make([]Parameter, 0),
			}

			// 提取参数
			for i := 0; i < len(args); i += 2 {
				if i+1 < len(args) {
					// 键值对参数
					if key, ok := args[i].(*ast.BasicLit); ok && key.Kind == token.STRING {
						paramName := strings.Trim(key.Value, "\"")
						paramType := getTypeString(args[i+1])
						event.Parameters = append(event.Parameters, Parameter{
							Name: paramName,
							Type: paramType,
						})
					}
				} else {
					// 单个参数
					paramType := getTypeString(args[i])
					event.Parameters = append(event.Parameters, Parameter{
						Name: fmt.Sprintf("param%d", i/2+1),
						Type: paramType,
					})
				}
			}

			events = append(events, event)
		}

		return true
	})

	return events
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
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.BasicLit:
		switch t.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "rune"
		default:
			return "unknown"
		}
	case *ast.StarExpr:
		return "*" + getTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + getTypeString(t.Elt)
		}
		return fmt.Sprintf("[%s]%s", t.Len, getTypeString(t.Elt))
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", getTypeString(t.X), t.Sel.Name)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", getTypeString(t.Key), getTypeString(t.Value))
	case *ast.ChanType:
		var dir string
		switch t.Dir {
		case ast.SEND:
			dir = "<-"
		case ast.RECV:
			dir = "<-"
		case ast.SEND | ast.RECV:
			dir = ""
		}
		return fmt.Sprintf("chan%s %s", dir, getTypeString(t.Value))
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.CallExpr:
		// For function calls, try to get the return type
		if sel, ok := t.Fun.(*ast.SelectorExpr); ok {
			return getTypeString(sel)
		}
		return "unknown"
	default:
		return fmt.Sprintf("%T", t)
	}
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
