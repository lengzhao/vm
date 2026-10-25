package vm

import (
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/compat"
)

type ContractCompiler = compat.ContractCompiler
type CompiledContract = core.CompiledContract
type ContractManager = compat.ContractManager
type ContractStatus = core.ContractStatus
type ContractMetadata = core.ContractMetadata
type VMConfig = core.VMConfig
type ExecuteResult = core.ExecuteResult
type EstimateOptions = core.EstimateOptions
type GasEstimate = core.GasEstimate
type ABIGenerator = core.ABIGenerator
type GasMetering = core.GasMetering
type GasLimitExceededError = core.GasLimitExceededError
type FunctionGasProfile = core.FunctionGasProfile
type GasProfile = core.GasProfile
type Address = core.Address
type Event = core.Event
type Host = core.Host
type CallContext = core.CallContext
type MemoryHost = core.MemoryHost
type ErrorCode = core.ErrorCode
type ExecutionError = core.ExecutionError
type CallRequest = core.CallRequest
type CallResult = core.CallResult
type Runner = core.Runner
type SecurityReviewer = compat.SecurityReviewer
type SecurityError = core.SecurityError
type SecurityErrorType = core.SecurityErrorType

const ContractStatusUnknown = core.ContractStatusUnknown
const ContractStatusDeployed = core.ContractStatusDeployed
const ContractStatusSuspended = core.ContractStatusSuspended
const ContractStatusDestroyed = core.ContractStatusDestroyed
const ProtocolVersion = core.ProtocolVersion
const CodeInvalidRequest = core.CodeInvalidRequest
const CodeInvalidArguments = core.CodeInvalidArguments
const CodeUnknownFunction = core.CodeUnknownFunction
const CodeContractError = core.CodeContractError
const CodeContractPanic = core.CodeContractPanic
const CodeGasLimitExceeded = core.CodeGasLimitExceeded
const CodeUnsupportedVersion = core.CodeUnsupportedVersion
const CodeInvalidResponse = core.CodeInvalidResponse
const CodeInvalidResult = core.CodeInvalidResult
const CodeTimeout = core.CodeTimeout
const CodeCanceled = core.CodeCanceled
const CodeOutputLimit = core.CodeOutputLimit
const CodeProcessFailure = core.CodeProcessFailure
const CodeUnsupportedABI = core.CodeUnsupportedABI
const ImportNotAllowed = core.ImportNotAllowed
const KeywordNotAllowed = core.KeywordNotAllowed
const GlobalVarNotAllowed = core.GlobalVarNotAllowed

func Uint64Ptr(v uint64) *uint64    { return core.Uint64Ptr(v) }
func AddressPtr(v Address) *Address { return core.AddressPtr(v) }
