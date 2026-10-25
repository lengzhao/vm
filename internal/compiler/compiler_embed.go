package compiler

import (
	_ "embed"
	"github.com/lengzhao/vm/contractapi"
)

var embeddedContractAPISource = contractapi.Source
var embeddedContractAPIGasSource = contractapi.GasSource
var embeddedContractAPIProtocolSource = contractapi.ProtocolSource

const compilerBuildID = "pipeline-v9-policy-guarantees"

//go:embed entry.go.tmpl
var contractEntryTemplate string
