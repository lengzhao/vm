package vm

import (
	_ "embed"
)

//go:embed contractapi/contractapi.go
var embeddedContractAPISource string

//go:embed contractapi/gas.go
var embeddedContractAPIGasSource string

// compilerBuildID 参与产物缓存指纹；变更编译管线时递增。
const compilerBuildID = "pipeline-v4-gas-profile"
