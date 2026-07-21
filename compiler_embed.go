package vm

import (
	_ "embed"
)

//go:embed contractapi/contractapi.go
var embeddedContractAPISource string

// compilerBuildID 参与产物缓存指纹；变更编译管线时递增。
const compilerBuildID = "pipeline-v3-embed-contractapi"
