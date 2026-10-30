package core

import "github.com/lengzhao/vm/contractapi"

const ProtocolVersion = contractapi.ProtocolVersion

type ErrorCode = contractapi.ErrorCode

const (
	CodeInvalidRequest     = contractapi.CodeInvalidRequest
	CodeInvalidArguments   = contractapi.CodeInvalidArguments
	CodeUnknownFunction    = contractapi.CodeUnknownFunction
	CodeContractError      = contractapi.CodeContractError
	CodeContractPanic      = contractapi.CodeContractPanic
	CodeGasLimitExceeded   = contractapi.CodeGasLimitExceeded
	CodeUnsupportedVersion = contractapi.CodeUnsupportedVersion
	CodeInvalidResponse    = contractapi.CodeInvalidResponse
	CodeInvalidResult      = contractapi.CodeInvalidResult
	CodeTimeout            = contractapi.CodeTimeout
	CodeCanceled           = contractapi.CodeCanceled
	CodeOutputLimit        = contractapi.CodeOutputLimit
	CodeProcessFailure     = contractapi.CodeProcessFailure
	CodeUnsupportedABI     = contractapi.CodeUnsupportedABI
)

// ExecutionError carries stable machine-readable classification and a diagnostic cause.
type ExecutionError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *ExecutionError) Error() string { return string(e.Code) + ": " + e.Message }

func (e *ExecutionError) Unwrap() error { return e.Cause }
