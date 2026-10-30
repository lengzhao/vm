// Legacy regression coverage exercises the real root compatibility facade.
package vm

import (
	compatvm "github.com/lengzhao/vm"
	"github.com/lengzhao/vm/internal/abigen"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"github.com/lengzhao/vm/internal/compat"
	compilerimpl "github.com/lengzhao/vm/internal/compiler"
	"github.com/lengzhao/vm/internal/gas"
	"github.com/lengzhao/vm/internal/policy"
	"github.com/lengzhao/vm/internal/security"
	"testing"
	"time"
)

type ContractCompiler = compatvm.ContractCompiler
type CompiledContract = compatvm.CompiledContract
type ContractManager = compatvm.ContractManager
type ContractStatus = compatvm.ContractStatus
type ContractMetadata = compatvm.ContractMetadata
type VMConfig = compatvm.VMConfig
type ExecuteResult = compatvm.ExecuteResult
type EstimateOptions = compatvm.EstimateOptions
type GasEstimate = compatvm.GasEstimate
type ABIGenerator = compatvm.ABIGenerator
type GasMetering = compatvm.GasMetering
type GasLimitExceededError = compatvm.GasLimitExceededError
type FunctionGasProfile = compatvm.FunctionGasProfile
type GasProfile = compatvm.GasProfile
type Address = compatvm.Address
type Event = compatvm.Event
type Host = compatvm.Host
type CallContext = compatvm.CallContext
type MemoryHost = compatvm.MemoryHost
type ErrorCode = compatvm.ErrorCode
type ExecutionError = compatvm.ExecutionError
type CallRequest = compatvm.CallRequest
type CallResult = compatvm.CallResult
type Runner = compatvm.Runner
type SecurityReviewer = compatvm.SecurityReviewer
type SecurityError = compatvm.SecurityError
type SecurityErrorType = compatvm.SecurityErrorType

const ContractStatusUnknown = compatvm.ContractStatusUnknown
const ContractStatusDeployed = compatvm.ContractStatusDeployed
const ContractStatusSuspended = compatvm.ContractStatusSuspended
const ContractStatusDestroyed = compatvm.ContractStatusDestroyed
const ProtocolVersion = compatvm.ProtocolVersion
const CodeInvalidRequest = compatvm.CodeInvalidRequest
const CodeInvalidArguments = compatvm.CodeInvalidArguments
const CodeUnknownFunction = compatvm.CodeUnknownFunction
const CodeContractError = compatvm.CodeContractError
const CodeContractPanic = compatvm.CodeContractPanic
const CodeGasLimitExceeded = compatvm.CodeGasLimitExceeded
const CodeUnsupportedVersion = compatvm.CodeUnsupportedVersion
const CodeInvalidResponse = compatvm.CodeInvalidResponse
const CodeInvalidResult = compatvm.CodeInvalidResult
const CodeTimeout = compatvm.CodeTimeout
const CodeCanceled = compatvm.CodeCanceled
const CodeOutputLimit = compatvm.CodeOutputLimit
const CodeProcessFailure = compatvm.CodeProcessFailure
const CodeUnsupportedABI = compatvm.CodeUnsupportedABI
const ImportNotAllowed = compatvm.ImportNotAllowed
const KeywordNotAllowed = compatvm.KeywordNotAllowed
const GlobalVarNotAllowed = compatvm.GlobalVarNotAllowed

var Uint64Ptr = compatvm.Uint64Ptr
var AddressPtr = compatvm.AddressPtr

type ContractCompilerImpl = compatvm.ContractCompilerImpl
type ContractManagerImpl = compatvm.ContractManagerImpl
type SecurityReviewerImpl = compatvm.SecurityReviewerImpl
type ABIGeneratorImpl = compatvm.ABIGeneratorImpl
type GasMeteringImpl = compatvm.GasMeteringImpl
type ProcessRunner = compatvm.ProcessRunner

const DeterministicPolicyVersion = compatvm.DeterministicPolicyVersion

type VMEngine = compatvm.VMEngine

var OpenVMEngine = compatvm.OpenVMEngine
var NewVMEngine = compatvm.NewVMEngine
var NewVMEngineWithRunner = compatvm.NewVMEngineWithRunner
var NewContractCompiler = compatvm.NewContractCompiler
var NewContractCompilerWithOptions = compatvm.NewContractCompilerWithOptions
var NewContractCompilerWithPolicy = compatvm.NewContractCompilerWithPolicy
var NewSecurityReviewer = compatvm.NewSecurityReviewer
var NewABIGenerator = compatvm.NewABIGenerator
var NewGasMetering = compatvm.NewGasMetering
var NewProcessRunner = compatvm.NewProcessRunner
var NewContractManager = compatvm.NewContractManager
var ReviewDeterministic = compatvm.ReviewDeterministic
var BuildGasProfile = compatvm.BuildGasProfile
var EstimateFromProfile = compatvm.EstimateFromProfile
var WithGasLimit = compatvm.WithGasLimit
var WithCallContext = compatvm.WithCallContext
var CallContextFrom = compatvm.CallContextFrom

func compilerWithTimeout(dir string, timeout time.Duration) ContractCompiler {
	p, err := policy.NewSource(policy.Options{EnableSecurity: true}, security.New())
	if err != nil {
		panic(err)
	}
	i := &gas.Injector{}
	c, err := compilerimpl.New(compilerimpl.Options{OutputDir: dir, BuildTimeout: timeout}, compilerimpl.Deps{Policy: p, ABI: abigen.New(), Injector: i, Profiler: &gas.Profiler{}})
	if err != nil {
		panic(err)
	}
	return compat.NewCompiler(c, p, i)
}

func sealedTestArtifact(t testing.TB, c *CompiledContract) *CompiledContract {
	return artifacttest.Seal(t, c)
}
