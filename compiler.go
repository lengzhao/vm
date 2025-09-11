package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"time"

	"github.com/lengzhao/vm/abi"
)

// ContractCompiler 编译器模块接口
// 根据简化设计原则，接口已精简为核心功能
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
	// 合约可执行文件路径
	ExecutablePath string

	// ABI信息
	ABI *abi.ABI

	// 编译时间
	CompileTime time.Time

	// 源代码哈希
	SourceHash string

	// 合约地址
	Address string
}

// ContractCompilerImpl 编译器模块实现
type ContractCompilerImpl struct {
	// 安全审查模块
	securityReviewer SecurityReviewer

	// ABI生成模块
	abiGenerator ABIGenerator

	// Gas消耗基础值
	baseGasConsumption uint64
}

// NewContractCompiler 创建新的编译器模块实例
func NewContractCompiler() ContractCompiler {
	return &ContractCompilerImpl{
		securityReviewer:   NewSecurityReviewer(),
		abiGenerator:       NewABIGenerator(),
		baseGasConsumption: 1, // 每行代码消耗1个Gas
	}
}

// Compile 编译源代码
func (c *ContractCompilerImpl) Compile(sourceCode string) (*CompiledContract, error) {
	// 验证源代码
	if err := c.Validate(sourceCode); err != nil {
		return nil, err
	}

	// 注入Gas计费代码
	gasInjectedCode, err := c.InjectGas(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to inject gas: %w", err)
	}

	// 生成带main函数的代码
	mainCode, err := c.generateMainFunction(gasInjectedCode)
	if err != nil {
		return nil, fmt.Errorf("failed to generate main function: %w", err)
	}

	// 生成ABI（基于原始代码，不包含main函数）
	contractABI, err := c.abiGenerator.Generate(gasInjectedCode)
	if err != nil {
		return nil, err
	}

	// 生成源代码哈希
	hash := generateHash(mainCode)

	// 创建编译后的合约对象
	compiledContract := &CompiledContract{
		ExecutablePath: "", // 实际编译过程将在后续实现
		ABI:            contractABI,
		CompileTime:    time.Now(),
		SourceHash:     hash,
		Address:        "", // 合约地址将在部署时设置
	}

	return compiledContract, nil
}

// generateMainFunction 生成Main函数
// 注意：此函数仅供框架内部使用，不应在合约源码中包含main函数
func (c *ContractCompilerImpl) generateMainFunction(sourceCode string) (string, error) {
	// 智能合约不应包含main函数，框架会在编译时自动生成
	// 检查源代码是否包含main函数
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", sourceCode, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("failed to parse source code: %w", err)
	}

	// 检查是否已存在main函数
	for _, decl := range file.Decls {
		if funcDecl, ok := decl.(*ast.FuncDecl); ok {
			if funcDecl.Name.Name == "main" {
				return "", fmt.Errorf("contract source code should not contain main function")
			}
		}
	}

	// 生成Main函数代码
	mainFunc := `
func main() {
	// 合约入口点由框架自动生成
	// 不应在合约源码中手动定义main函数
}
`

	// 在源代码末尾添加Main函数
	modifiedCode := sourceCode + mainFunc

	return modifiedCode, nil
}

// Validate 验证源代码
func (c *ContractCompilerImpl) Validate(sourceCode string) error {
	// 使用安全审查模块进行验证
	return c.securityReviewer.Review(sourceCode)
}

// InjectGas 注入Gas计费代码
func (c *ContractCompilerImpl) InjectGas(sourceCode string) (string, error) {
	// 简化实现：在源代码中添加注释表示Gas注入
	// 在实际实现中，这里会进行AST分析和代码注入
	injectedCode := "// Gas-injected code\n" + sourceCode
	return injectedCode, nil
}

// generateHash 生成源代码的哈希值
func generateHash(sourceCode string) string {
	hash := sha256.Sum256([]byte(sourceCode))
	return hex.EncodeToString(hash[:])[:16]
}
