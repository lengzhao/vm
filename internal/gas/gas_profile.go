package gas

import (
	"fmt"
	"github.com/lengzhao/vm/contractapi"
	"github.com/lengzhao/vm/core"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"
)

// 已知 API 名称 → 单次调用 Gas（与 contractapi 常量一致）。
var gasAPITable = map[string]uint64{
	"BlockHeight":     contractapi.GasBlockHeight,
	"BlockTime":       contractapi.GasBlockTime,
	"Sender":          contractapi.GasSender,
	"ContractAddress": contractapi.GasContractAddr,
	"Log":             contractapi.GasLog,
	"Call":            contractapi.GasCrossCall,
	"Object.Get":      contractapi.GasObjectGet,
	"Object.Set":      contractapi.GasObjectSet,
	"Object.Delete":   contractapi.GasObjectDelete,
}

// BuildGasProfile 在注入前对原始合约源码做 AST 扫描，生成 core.GasProfile。
func BuildGasProfile(source string) (*core.GasProfile, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", source, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	pkgFuncs := packageFuncNames(file)
	apiPkg := contractapiImportName(file)

	profile := &core.GasProfile{
		Version:   1,
		EntryGas:  contractapi.GasEntryBase,
		Functions: make(map[string]core.FunctionGasProfile),
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || !fn.Name.IsExported() {
			continue
		}
		profile.Functions[fn.Name.Name] = scanFunction(fn, pkgFuncs, apiPkg)
	}
	return profile, nil
}

// EstimateFromProfile 按静态公式估算指定函数的 Gas 启发式值，并返回分项 breakdown。
func EstimateFromProfile(p *core.GasProfile, function string, loopBound uint64) (uint64, map[string]uint64, error) {
	if p == nil {
		return 0, nil, fmt.Errorf("nil gas profile")
	}
	if p.Version != 1 {
		return 0, nil, fmt.Errorf("unsupported gas profile version %d; require version 1", p.Version)
	}
	fp, ok := p.Functions[function]
	if !ok {
		return 0, nil, fmt.Errorf("function %q not found in gas profile", function)
	}
	if loopBound == 0 {
		loopBound = 1000
	}

	entry := p.EntryGas
	funcs := uint64(fp.FuncEntries) * contractapi.GasFuncOrLoop
	loops := uint64(fp.LoopSites) * loopBound

	var api uint64
	for name, count := range fp.APICalls {
		cost, known := gasAPITable[name]
		if !known {
			continue
		}
		api += uint64(count) * cost
	}

	breakdown := map[string]uint64{
		"entry": entry,
		"funcs": funcs,
		"loops": loops,
		"api":   api,
	}
	return entry + funcs + loops + api, breakdown, nil
}

func packageFuncNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil {
			continue
		}
		names[fn.Name.Name] = true
	}
	return names
}

func contractapiImportName(file *ast.File) string {
	for _, imp := range file.Imports {
		pkgPath := strings.Trim(imp.Path.Value, `"`)
		if path.Base(pkgPath) != "contractapi" {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				return ""
			}
			return imp.Name.Name
		}
		return "contractapi"
	}
	return ""
}

func scanFunction(fn *ast.FuncDecl, pkgFuncs map[string]bool, apiPkg string) core.FunctionGasProfile {
	called := make(map[string]bool)
	apiCalls := make(map[string]int)
	loopSites := 0

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			loopSites++
		case *ast.CallExpr:
			collectDirectCall(x, fn.Name.Name, pkgFuncs, apiPkg, called, apiCalls)
		}
		return true
	})

	entries := 1 + len(called)
	return core.FunctionGasProfile{
		FuncEntries: entries,
		LoopSites:   loopSites,
		APICalls:    apiCalls,
	}
}

func collectDirectCall(
	call *ast.CallExpr,
	self string,
	pkgFuncs map[string]bool,
	apiPkg string,
	called map[string]bool,
	apiCalls map[string]int,
) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		name := fun.Name
		if name != self && pkgFuncs[name] {
			called[name] = true
		}
	case *ast.SelectorExpr:
		if apiPkg == "" {
			return
		}
		// Without full type information, charge matching method names whenever
		// contractapi is imported. This may overcount unrelated methods; indirect
		// function-value calls remain outside this heuristic profile.
		if _, known := gasAPITable["Object."+fun.Sel.Name]; known {
			apiCalls["Object."+fun.Sel.Name]++
			return
		}
		pkgIdent, ok := fun.X.(*ast.Ident)
		if !ok || pkgIdent.Name != apiPkg {
			return
		}
		apiCalls[fun.Sel.Name]++
	}
}
