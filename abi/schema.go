package abi

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

const SchemaVersion = 1

func IsScalarType(t string) bool {
	switch t {
	case "bool", "string", "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "byte", "rune", "float32", "float64":
		return true
	}
	return false
}

// ValidateCallable applies the one shared type contract used by API and compiler.
func ValidateCallable(a *ABI) error {
	if a == nil || a.Version != SchemaVersion {
		return fmt.Errorf("unsupported ABI version; require %d", SchemaVersion)
	}
	seen := map[string]bool{}
	for _, fn := range a.Functions {
		if seen[fn.Name] || fn.Name == "" {
			return fmt.Errorf("duplicate or empty function name %q", fn.Name)
		}
		seen[fn.Name] = true
		if fn.Generic {
			return fmt.Errorf("function %s: generic exports are unsupported", fn.Name)
		}
		for _, p := range fn.Inputs {
			if !IsScalarType(p.Type) {
				return fmt.Errorf("function %s: unsupported parameter type: %s", fn.Name, p.Type)
			}
		}
		for i, p := range fn.Outputs {
			if !IsScalarType(p.Type) && !(p.Type == "error" && i == len(fn.Outputs)-1) {
				return fmt.Errorf("function %s: unsupported return type: %s", fn.Name, p.Type)
			}
		}
	}
	return nil
}

// ExtractCallableABI rejects unsupported public signatures before code generation.
func ExtractCallableABI(code []byte) (*ABI, error) {
	a, err := ExtractABI(code)
	if err != nil {
		return nil, err
	}
	if err = ValidateCallable(a); err != nil {
		return nil, err
	}
	f, err := parser.ParseFile(token.NewFileSet(), "", code, 0)
	if err != nil {
		return nil, err
	}
	// A package declaration must not redefine scalar names used by the generated ABI.
	for _, d := range f.Decls {
		var names []string
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				names = append(names, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					names = append(names, s.Name.Name)
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names = append(names, n.Name)
					}
				}
			}
		}
		for _, n := range names {
			if reservedPredeclared(n) {
				return nil, fmt.Errorf("reserved predeclared name is shadowed: %s", n)
			}
		}
	}
	return a, nil
}

func reservedPredeclared(name string) bool {
	if IsScalarType(name) {
		return true
	}
	switch name {
	case "error", "uintptr", "complex64", "complex128", "any", "comparable", "true", "false", "nil", "iota", "append", "cap", "clear", "close", "complex", "copy", "delete", "imag", "len", "make", "max", "min", "new", "panic", "print", "println", "real", "recover":
		return true
	}
	return false
}
