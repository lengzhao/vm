package vm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lengzhao/vm/abi"
)

// ContractCompiler 编译器模块接口
type ContractCompiler interface {
	// Compile 编译源代码
	Compile(sourceCode string) (*CompiledContract, error)

	// Validate 验证源代码
	Validate(sourceCode string) error

	// InjectGas 注入Gas计费代码
	InjectGas(sourceCode string) (string, error)
}

// CompiledContract 编译后的合约
type CompiledContract struct {
	ExecutablePath string
	ABI            *abi.ABI
	GasProfile     *GasProfile
	CompileTime    time.Time
	SourceHash     string
	Address        string
}

// ContractCompilerImpl 编译器模块实现
type ContractCompilerImpl struct {
	securityReviewer   SecurityReviewer
	abiGenerator       ABIGenerator
	baseGasConsumption uint64
	outputDir          string
	enableSecurity     bool
}

// NewContractCompiler 创建新的编译器模块实例
func NewContractCompiler() ContractCompiler {
	return NewContractCompilerWithOptions("", true)
}

// NewContractCompilerWithOptions 创建带输出目录与安全开关的编译器
func NewContractCompilerWithOptions(outputDir string, enableSecurity bool) ContractCompiler {
	if outputDir == "" {
		outputDir = filepath.Join(os.TempDir(), "lengzhao-vm-build")
	}
	return &ContractCompilerImpl{
		securityReviewer:   NewSecurityReviewer(),
		abiGenerator:       NewABIGenerator(),
		baseGasConsumption: 1,
		outputDir:          outputDir,
		enableSecurity:     enableSecurity,
	}
}

// Compile 编译源代码
func (c *ContractCompilerImpl) Compile(sourceCode string) (*CompiledContract, error) {
	if strings.TrimSpace(sourceCode) == "" {
		return nil, fmt.Errorf("source code cannot be empty")
	}

	if err := c.Validate(sourceCode); err != nil {
		return nil, err
	}

	contractABI, err := c.abiGenerator.Generate(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ABI: %w", err)
	}

	gasProfile, err := BuildGasProfile(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to build gas profile: %w", err)
	}

	hash := generateBuildHash(sourceCode)
	execPath := filepath.Join(c.outputDir, "contract_"+hash)
	if cached, ok := loadCachedExecutable(execPath); ok {
		return &CompiledContract{
			ExecutablePath: cached,
			ABI:            contractABI,
			GasProfile:     gasProfile,
			CompileTime:    time.Now(),
			SourceHash:     hash,
			Address:        "",
		}, nil
	}

	gasInjectedCode, err := c.InjectGas(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to inject gas: %w", err)
	}

	if err := c.ensureNoMain(gasInjectedCode); err != nil {
		return nil, err
	}

	entryCode, err := c.generateEntryFile(contractABI)
	if err != nil {
		return nil, fmt.Errorf("failed to generate entry: %w", err)
	}

	builtPath, err := c.buildExecutable(hash, gasInjectedCode, entryCode)
	if err != nil {
		return nil, fmt.Errorf("failed to build contract: %w", err)
	}

	return &CompiledContract{
		ExecutablePath: builtPath,
		ABI:            contractABI,
		GasProfile:     gasProfile,
		CompileTime:    time.Now(),
		SourceHash:     hash,
		Address:        "",
	}, nil
}

func (c *ContractCompilerImpl) ensureNoMain(sourceCode string) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse source code: %w", err)
	}

	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok && funcDecl.Name.Name == "main" {
			return fmt.Errorf("contract source code should not contain main function")
		}
	}
	return nil
}

// generateEntryFile 生成合约入口与 Gas 辅助代码
func (c *ContractCompilerImpl) generateEntryFile(contractABI *abi.ABI) (string, error) {
	var b strings.Builder
	b.WriteString(`package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/lengzhao/vm/contractapi"
)

func __vmConsumeGas(amount uint64) {
	contractapi.ConsumeGas(amount)
}

type __vmCallRequest struct {
	Function string            ` + "`json:\"function\"`" + `
	Args     []json.RawMessage ` + "`json:\"args\"`" + `
}

type __vmCallResponse struct {
	OK     bool               ` + "`json:\"ok\"`" + `
	Result interface{}        ` + "`json:\"result,omitempty\"`" + `
	Error  string             ` + "`json:\"error,omitempty\"`" + `
	Gas    uint64             ` + "`json:\"gas\"`" + `
	Events []contractapi.Event ` + "`json:\"events,omitempty\"`" + `
}

func __vmWriteResponse(resp __vmCallResponse) {
	resp.Gas = contractapi.GasUsed()
	resp.Events = contractapi.DrainEvents()
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(resp)
}

func __vmParseInt(raw json.RawMessage) (int, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 != nil {
			return 0, err
		}
		v, err2 := strconv.Atoi(s)
		return v, err2
	}
	v, err := n.Int64()
	return int(v), err
}

func __vmParseInt64(raw json.RawMessage) (int64, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return n.Int64()
}

func __vmParseUint64(raw json.RawMessage) (uint64, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return strconv.ParseUint(string(n), 10, 64)
}

func __vmParseFloat64(raw json.RawMessage) (float64, error) {
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return n.Float64()
}

func __vmParseString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}

func __vmParseBool(raw json.RawMessage) (bool, error) {
	var v bool
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, err
	}
	return v, nil
}

func main() {
	contractapi.Reset()
	contractapi.ResetGas()

	var limit uint64
	if raw := os.Getenv("VM_GAS_LIMIT"); raw != "" {
		if v, err := strconv.ParseUint(raw, 10, 64); err == nil {
			limit = v
		}
	}
	contractapi.InitGas(limit)
	contractapi.ConsumeGas(contractapi.GasEntryBase)

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		__vmWriteResponse(__vmCallResponse{OK: false, Error: err.Error()})
		os.Exit(1)
	}

	var req __vmCallRequest
	if err := json.Unmarshal(data, &req); err != nil {
		__vmWriteResponse(__vmCallResponse{OK: false, Error: "invalid request json: " + err.Error()})
		os.Exit(1)
	}

	defer func() {
		if r := recover(); r != nil {
			__vmWriteResponse(__vmCallResponse{OK: false, Error: fmt.Sprint(r)})
			os.Exit(1)
		}
	}()

	switch req.Function {
`)

	if contractABI != nil {
		for _, fn := range contractABI.Functions {
			caseCode, err := generateFunctionCase(fn)
			if err != nil {
				return "", err
			}
			b.WriteString(caseCode)
		}
	}

	b.WriteString(`	default:
		__vmWriteResponse(__vmCallResponse{OK: false, Error: "unknown function: " + req.Function})
		os.Exit(1)
	}
}
`)

	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return b.String(), nil
	}
	return string(formatted), nil
}

func generateFunctionCase(fn abi.Function) (string, error) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\tcase %q:\n", fn.Name))

	argNames := make([]string, 0, len(fn.Inputs))
	for i, input := range fn.Inputs {
		name := input.Name
		if name == "" {
			name = fmt.Sprintf("arg%d", i)
		}
		safeName := sanitizeIdent(name)
		argNames = append(argNames, safeName)

		parser, goType, err := parserForType(input.Type)
		if err != nil {
			return "", fmt.Errorf("function %s: %w", fn.Name, err)
		}
		b.WriteString(fmt.Sprintf("\t\tif len(req.Args) <= %d {\n", i))
		b.WriteString(fmt.Sprintf("\t\t\t__vmWriteResponse(__vmCallResponse{OK: false, Error: \"missing arg %d for %s\"})\n", i, fn.Name))
		b.WriteString("\t\t\tos.Exit(1)\n")
		b.WriteString("\t\t}\n")
		b.WriteString(fmt.Sprintf("\t\t%s, err := %s(req.Args[%d])\n", safeName, parser, i))
		b.WriteString("\t\tif err != nil {\n")
		b.WriteString(fmt.Sprintf("\t\t\t__vmWriteResponse(__vmCallResponse{OK: false, Error: \"invalid arg %d (%s): \" + err.Error()})\n", i, goType))
		b.WriteString("\t\t\tos.Exit(1)\n")
		b.WriteString("\t\t}\n")
		_ = goType
	}

	callArgs := strings.Join(argNames, ", ")
	switch len(fn.Outputs) {
	case 0:
		b.WriteString(fmt.Sprintf("\t\t%s(%s)\n", fn.Name, callArgs))
		b.WriteString("\t\t__vmWriteResponse(__vmCallResponse{OK: true})\n")
	case 1:
		b.WriteString(fmt.Sprintf("\t\tresult := %s(%s)\n", fn.Name, callArgs))
		b.WriteString("\t\t__vmWriteResponse(__vmCallResponse{OK: true, Result: result})\n")
	default:
		outs := make([]string, len(fn.Outputs))
		for i := range fn.Outputs {
			outs[i] = fmt.Sprintf("out%d", i)
		}
		b.WriteString(fmt.Sprintf("\t\t%s := %s(%s)\n", strings.Join(outs, ", "), fn.Name, callArgs))
		b.WriteString(fmt.Sprintf("\t\t__vmWriteResponse(__vmCallResponse{OK: true, Result: []interface{}{%s}})\n", strings.Join(outs, ", ")))
	}
	return b.String(), nil
}

func sanitizeIdent(name string) string {
	name = strings.ReplaceAll(name, "-", "_")
	if name == "" || !((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z') || name[0] == '_') {
		return "arg_" + name
	}
	return name
}

func parserForType(typeName string) (parserName, goType string, err error) {
	switch typeName {
	case "int":
		return "__vmParseInt", "int", nil
	case "int64":
		return "__vmParseInt64", "int64", nil
	case "uint64":
		return "__vmParseUint64", "uint64", nil
	case "float64":
		return "__vmParseFloat64", "float64", nil
	case "string":
		return "__vmParseString", "string", nil
	case "bool":
		return "__vmParseBool", "bool", nil
	default:
		return "", "", fmt.Errorf("unsupported parameter type: %s", typeName)
	}
}

func (c *ContractCompilerImpl) buildExecutable(hash, contractCode, entryCode string) (string, error) {
	if err := os.MkdirAll(c.outputDir, 0755); err != nil {
		return "", err
	}

	execPath := filepath.Join(c.outputDir, "contract_"+hash)
	if cached, ok := loadCachedExecutable(execPath); ok {
		return cached, nil
	}

	buildDir := filepath.Join(c.outputDir, "build_"+hash)
	if err := os.RemoveAll(buildDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(buildDir, "contractapi"), 0755); err != nil {
		return "", err
	}

	normalized, err := ensurePackageMain(contractCode)
	if err != nil {
		return "", err
	}

	contractPath := filepath.Join(buildDir, "contract.go")
	entryPath := filepath.Join(buildDir, "entry.go")
	apiPath := filepath.Join(buildDir, "contractapi", "contractapi.go")
	apiGasPath := filepath.Join(buildDir, "contractapi", "gas.go")
	apiModPath := filepath.Join(buildDir, "contractapi", "go.mod")
	modPath := filepath.Join(buildDir, "go.mod")

	if err := os.WriteFile(contractPath, []byte(normalized), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(entryPath, []byte(entryCode), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(apiPath, []byte(embeddedContractAPISource), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(apiGasPath, []byte(embeddedContractAPIGasSource), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(apiModPath, []byte("module github.com/lengzhao/vm/contractapi\n\ngo 1.22\n"), 0644); err != nil {
		return "", err
	}

	modContent := fmt.Sprintf(`module contract_%s

go 1.22

require github.com/lengzhao/vm/contractapi v0.0.0

replace github.com/lengzhao/vm/contractapi => ./contractapi
`, hash)
	if err := os.WriteFile(modPath, []byte(modContent), 0644); err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-mod=mod", "-o", execPath, ".")
	cmd.Dir = buildDir
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		slog.Error("contract build failed", "hash", hash, "stderr", stderr.String())
		return "", fmt.Errorf("go build failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return execPath, nil
}

func loadCachedExecutable(execPath string) (string, bool) {
	info, err := os.Stat(execPath)
	if err != nil || info.IsDir() {
		return "", false
	}
	if info.Mode()&0o111 == 0 {
		return "", false
	}
	return execPath, true
}

func generateBuildHash(sourceCode string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(compilerBuildID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(embeddedContractAPISource))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(embeddedContractAPIGasSource))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(sourceCode))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func ensurePackageMain(sourceCode string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.ParseComments)
	if err != nil {
		return "", err
	}
	file.Name = ast.NewIdent("main")
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Validate 验证源代码
func (c *ContractCompilerImpl) Validate(sourceCode string) error {
	if !c.enableSecurity {
		fset := token.NewFileSet()
		_, err := parser.ParseFile(fset, "", sourceCode, parser.AllErrors)
		return err
	}
	return c.securityReviewer.Review(sourceCode)
}

// InjectGas 在函数入口和循环体注入 Gas 消耗点
func (c *ContractCompilerImpl) InjectGas(sourceCode string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.ParseComments)
	if err != nil {
		return "", err
	}

	amount := c.baseGasConsumption
	if amount == 0 {
		amount = 1
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Body == nil || x.Name.Name == "main" {
				return true
			}
			x.Body.List = append([]ast.Stmt{consumeGasStmt(amount)}, x.Body.List...)
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
