package compiler

import (
	_ "embed"
	"github.com/lengzhao/vm/contractapi"
)

var embeddedContractAPISource = contractapi.Source
var embeddedContractAPIGasSource = contractapi.GasSource
var embeddedContractAPIProtocolSource = contractapi.ProtocolSource

var embeddedContractAPIObjectSource = contractapi.ObjectSource

var embeddedContractAPICallSource = contractapi.CallSource

const compilerBuildID = "pipeline-v13-cross-contract"

//go:embed entry.go.tmpl
var contractEntryTemplate string
