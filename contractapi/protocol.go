package contractapi

// ProtocolVersion is the only supported wire protocol.
const ProtocolVersion = 3

type ErrorCode string

const (
	CodeCallFailure        ErrorCode = "CALL_FAILURE"
	CodeInvalidRequest     ErrorCode = "INVALID_REQUEST"
	CodeInvalidArguments   ErrorCode = "INVALID_ARGUMENTS"
	CodeUnknownFunction    ErrorCode = "UNKNOWN_FUNCTION"
	CodeContractError      ErrorCode = "CONTRACT_ERROR"
	CodeContractPanic      ErrorCode = "CONTRACT_PANIC"
	CodeGasLimitExceeded   ErrorCode = "GAS_LIMIT_EXCEEDED"
	CodeUnsupportedVersion ErrorCode = "UNSUPPORTED_VERSION"
	CodeInvalidResponse    ErrorCode = "INVALID_RESPONSE"
	CodeInvalidResult      ErrorCode = "INVALID_RESULT"
	CodeTimeout            ErrorCode = "EXECUTION_TIMEOUT"
	CodeCanceled           ErrorCode = "EXECUTION_CANCELED"
	CodeOutputLimit        ErrorCode = "OUTPUT_LIMIT_EXCEEDED"
	CodeProcessFailure     ErrorCode = "PROCESS_FAILURE"
	CodeUnsupportedABI     ErrorCode = "UNSUPPORTED_ABI"
)

// GasLimitError is runtime-owned, so user panic strings cannot impersonate OOG.
type GasLimitError struct{}

func (GasLimitError) Error() string { return "gas limit exceeded" }
