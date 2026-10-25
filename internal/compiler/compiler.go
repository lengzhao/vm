package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Compiler 编译器模块实现
type Compiler struct {
	goBinary     string
	goVersion    string
	abiGenerator core.ABIGenerator
	outputDir    string
	buildTimeout time.Duration
	policy       core.SourcePolicy
	injector     core.GasInjector
	profiler     core.GasProfileBuilder
}

type Options struct {
	GoBinary           string
	GoVersion          string
	OutputDir          string
	BuildTimeout       time.Duration
	RequireDeterminism bool
}
type Deps struct {
	Policy   core.SourcePolicy
	ABI      core.ABIGenerator
	Injector core.GasInjector
	Profiler core.GasProfileBuilder
}

// New constructs a compiler exclusively from explicit dependencies.
func New(c Options, d Deps) (*Compiler, error) {
	if d.Policy == nil || d.ABI == nil || d.Injector == nil || d.Profiler == nil {
		return nil, fmt.Errorf("compiler dependencies required")
	}
	if c.RequireDeterminism && !core.GuaranteesOf(d.Policy).Deterministic {
		return nil, fmt.Errorf("deterministic compiler requires deterministic source policy")
	}
	if d.Policy.ID() == "" {
		return nil, fmt.Errorf("source policy ID required")
	}
	if c.OutputDir == "" {
		c.OutputDir = filepath.Join(os.TempDir(), "lengzhao-vm-build")
	}
	if c.BuildTimeout <= 0 {
		c.BuildTimeout = 30 * time.Second
	}
	if c.GoBinary == "" {
		c.GoBinary = "go"
	}
	if c.GoVersion == "" {
		c.GoVersion = runtime.Version()
	}
	return &Compiler{goBinary: c.GoBinary, goVersion: c.GoVersion, outputDir: c.OutputDir, buildTimeout: c.BuildTimeout, policy: d.Policy, abiGenerator: d.ABI, injector: d.Injector, profiler: d.Profiler}, nil
}

// Compile 编译源代码
func (c *Compiler) Compile(ctx context.Context, sourceCode string) (*core.CompiledContract, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(sourceCode) == "" {
		return nil, fmt.Errorf("source code cannot be empty")
	}

	if err := c.Validate(sourceCode); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contractABI, err := c.abiGenerator.Generate(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ABI: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	gasProfile, err := c.profiler.BuildProfile(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to build gas profile: %w", err)
	}

	outputDir, err := filepath.Abs(c.outputDir)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	gasInjectedCode, err := c.InjectGas(sourceCode)
	if err != nil {
		return nil, fmt.Errorf("failed to inject gas: %w", err)
	}

	if err := c.ensureNoMain(gasInjectedCode); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entryCode, err := c.generateEntryFile(contractABI)
	if err != nil {
		return nil, fmt.Errorf("failed to generate entry: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	buildCtx, cancelBuild := context.WithTimeout(ctx, c.buildTimeout)
	defer cancelBuild()
	tc, err := c.resolveToolchain(buildCtx)
	if err != nil {
		return nil, err
	}
	build := core.ArtifactBuild{
		CompilerVersion: compilerBuildID,
		Toolchain:       core.ArtifactToolchain{Version: c.goVersion, BinarySHA256: tc.binarySHA256, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Flags: []string{"-mod=mod", "-trimpath", "-buildvcs=false", "-ldflags=-buildid="}},
		RuntimeVersion:  core.ArtifactRuntimeVersion, GasVersion: core.ArtifactGasVersion,
		RuntimeSHA256: artifact.RuntimeDigest(), SourceSHA256: artifact.Digest([]byte(sourceCode)),
		InjectedSHA256: artifact.Digest([]byte(gasInjectedCode)), EntrySHA256: artifact.Digest([]byte(entryCode)),
	}
	identity, err := json.Marshal(struct {
		Build      core.ArtifactBuild
		ABI        *abi.ABI
		Profile    *core.GasProfile
		Policy     string
		Guarantees core.PolicyGuarantees
	}{build, contractABI, gasProfile, c.sourcePolicy(), core.GuaranteesOf(c.policy)})
	if err != nil {
		return nil, err
	}
	hash := generateBuildHash(string(identity))
	record := &core.CompiledContract{SourcePolicy: c.sourcePolicy(), Guarantees: core.GuaranteesOf(c.policy), ProtocolVersion: core.ProtocolVersion, ABI: contractABI, GasProfile: gasProfile, CompileTime: time.Now(), SourceHash: hash}
	cacheDir := filepath.Join(outputDir, "contract_"+hash)
	if cached, ok, err := loadCachedArtifact(buildCtx, cacheDir, record, build); ok || err != nil {
		return cached, err
	}
	return c.buildExecutable(buildCtx, tc, record, build, gasInjectedCode, entryCode)
}

func (c *Compiler) ensureNoMain(sourceCode string) error {
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
func (c *Compiler) generateEntryFile(contractABI *abi.ABI) (string, error) {
	if err := abi.ValidateCallable(contractABI); err != nil {
		return "", &core.ExecutionError{Code: core.CodeUnsupportedABI, Message: err.Error(), Cause: err}
	}
	var b strings.Builder
	b.WriteString(contractEntryTemplate)

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
		__vmFail(__vmAPI.CodeUnknownFunction, "unknown function: " + req.Function)
	}
}
`)

	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return "", fmt.Errorf("invalid generated entry: %w", err)
	}
	return string(formatted), nil
}

func generateFunctionCase(fn abi.Function) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "case %q:\n", fn.Name)
	fmt.Fprintf(&b, "if len(req.Args) != %d { __vmFail(__vmAPI.CodeInvalidArguments, %q) }\n", len(fn.Inputs), "wrong argument count for "+fn.Name)
	args := make([]string, len(fn.Inputs))
	for i, input := range fn.Inputs {
		parser, _, err := parserForType(input.Type)
		if err != nil {
			return "", err
		}
		args[i] = fmt.Sprintf("__vmArg%d", i)
		fmt.Fprintf(&b, "%s, __vmArgErr%d := %s(req.Args[%d])\n", args[i], i, parser, i)
		fmt.Fprintf(&b, "if __vmArgErr%d != nil { __vmFail(__vmAPI.CodeInvalidArguments, %q + __vmArgErr%d.Error()) }\n", i, fmt.Sprintf("invalid arg %d (%s): ", i, input.Type), i)
	}
	outputs := make([]string, len(fn.Outputs))
	for i := range outputs {
		outputs[i] = fmt.Sprintf("__vmOut%d", i)
	}
	if len(outputs) > 0 {
		fmt.Fprintf(&b, "%s := ", strings.Join(outputs, ", "))
	}
	fmt.Fprintf(&b, "%s(%s)\n", fn.Name, strings.Join(args, ", "))
	if len(outputs) > 0 && fn.Outputs[len(outputs)-1].Type == "error" {
		last := outputs[len(outputs)-1]
		fmt.Fprintf(&b, "if %s != nil { __vmFail(__vmAPI.CodeContractError, %s.Error()) }\n", last, last)
		outputs = outputs[:len(outputs)-1]
	}
	result := "nil"
	if len(outputs) == 1 {
		result = outputs[0]
	} else if len(outputs) > 1 {
		result = "[]interface{}{" + strings.Join(outputs, ", ") + "}"
	}
	fmt.Fprintf(&b, "__vmWriteResponse(__vmCallResponse{OK:true,Result:%s})\n", result)
	return b.String(), nil
}

func parserForType(t string) (string, string, error) {
	if !abi.IsScalarType(t) {
		return "", "", fmt.Errorf("unsupported parameter type: %s", t)
	}
	return "__vmDecodeScalar[" + t + "]", t, nil
}

func (c *Compiler) buildExecutable(parent context.Context, tc toolchain, record *core.CompiledContract, build core.ArtifactBuild, contractCode, entryCode string) (*core.CompiledContract, error) {
	hash := record.SourceHash
	outputDir, err := filepath.Abs(c.outputDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	cacheDir := filepath.Join(outputDir, "contract_"+hash)
	if cached, ok, err := loadCachedArtifact(parent, cacheDir, record, build); ok || err != nil {
		return cached, err
	}

	buildDir, err := os.MkdirTemp(outputDir, ".build_"+hash+"-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(buildDir)
	if err := os.MkdirAll(filepath.Join(buildDir, "contractapi"), 0755); err != nil {
		return nil, err
	}

	normalized, err := ensurePackageMain(contractCode)
	if err != nil {
		return nil, err
	}

	contractPath := filepath.Join(buildDir, "contract.go")
	entryPath := filepath.Join(buildDir, "entry.go")
	apiPath := filepath.Join(buildDir, "contractapi", "contractapi.go")
	apiGasPath := filepath.Join(buildDir, "contractapi", "gas.go")
	apiProtocolPath := filepath.Join(buildDir, "contractapi", "protocol.go")
	apiModPath := filepath.Join(buildDir, "contractapi", "go.mod")
	modPath := filepath.Join(buildDir, "go.mod")

	if err := os.WriteFile(contractPath, []byte(normalized), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(entryPath, []byte(entryCode), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(apiPath, []byte(embeddedContractAPISource), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(apiGasPath, []byte(embeddedContractAPIGasSource), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(apiProtocolPath, []byte(embeddedContractAPIProtocolSource), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(apiModPath, []byte("module github.com/lengzhao/vm/contractapi\n\ngo 1.22\n"), 0644); err != nil {
		return nil, err
	}

	modContent := fmt.Sprintf(`module contract_%s

go 1.22

require github.com/lengzhao/vm/contractapi v0.0.0

replace github.com/lengzhao/vm/contractapi => ./contractapi
`, hash)
	if err := os.WriteFile(modPath, []byte(modContent), 0644); err != nil {
		return nil, err
	}

	timeout := c.buildTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	publishDir := filepath.Join(buildDir, "publish")
	if err := os.Mkdir(publishDir, 0755); err != nil {
		return nil, err
	}
	executable := filepath.Join(publishDir, "artifact")
	cmd := exec.CommandContext(ctx, tc.binary, "build", "-mod=mod", "-o", executable, "-trimpath", "-buildvcs=false", "-ldflags=-buildid=", ".")
	cmd.WaitDelay = 100 * time.Millisecond
	cmd.Dir = buildDir
	cmd.Env = tc.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("contract build: %w", ctx.Err())
		}
		slog.Error("contract build failed", "hash", hash, "stderr", stderr.String())
		return nil, fmt.Errorf("go build failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record.ExecutablePath = executable
	if err := artifact.Seal(ctx, record, build); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(record.Manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(publishDir, "manifest.json"), manifest, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(publishDir, "manifest.sha256"), []byte(record.ManifestSHA256), 0644); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Rename(publishDir, cacheDir); err != nil {
		// Another compiler may have published the same complete cache entry.
		if cached, ok, loadErr := loadCachedArtifact(ctx, cacheDir, record, build); ok || loadErr != nil {
			return cached, loadErr
		}
		return nil, fmt.Errorf("publish artifact: %w", err)
	}
	record.ExecutablePath = filepath.Join(cacheDir, "artifact")
	return record, nil
}

func loadCachedArtifact(ctx context.Context, dir string, expected *core.CompiledContract, build core.ArtifactBuild) (*core.CompiledContract, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "invalid artifact cache directory; remove cache and recompile"}
	}
	data, err := readCacheRecord(ctx, filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, false, &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "read cached manifest", Cause: err}
	}
	var manifest core.ArtifactManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, false, &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "decode cached manifest", Cause: err}
	}
	record := *expected
	record.ExecutablePath = filepath.Join(dir, "artifact")
	record.Manifest = &manifest
	checksum, err := readCacheRecord(ctx, filepath.Join(dir, "manifest.sha256"))
	if err != nil {
		return nil, false, &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "read cached manifest checksum", Cause: err}
	}
	record.ManifestSHA256 = string(checksum)
	if err := artifact.Validate(ctx, &record); err != nil {
		return nil, false, err
	}
	got, _ := artifact.JSONDigest(manifest.Build)
	want, _ := artifact.JSONDigest(build)
	if got != want {
		return nil, false, &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "cached build manifest mismatch"}
	}
	return &record, true, nil
}

func readCacheRecord(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("cache record must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err == nil {
		err = ctx.Err()
	}
	return data, err
}

func generateBuildHash(sourceCode string) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s", compilerBuildID, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(embeddedContractAPISource))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(embeddedContractAPIGasSource))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(embeddedContractAPIProtocolSource))
	_, _ = h.Write([]byte(contractEntryTemplate))
	_, _ = h.Write([]byte(sourceCode))
	return hex.EncodeToString(h.Sum(nil))
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
func (c *Compiler) Validate(sourceCode string) error { return c.policy.Review(sourceCode) }

// InjectGas 在函数入口和循环体注入 Gas 消耗点
func (c *Compiler) InjectGas(sourceCode string) (string, error) {
	return c.injector.InjectGas(sourceCode)
}

func (c *Compiler) sourcePolicy() string { return c.policy.ID() }
