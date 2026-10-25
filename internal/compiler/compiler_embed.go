package compiler

import (
	_ "embed"
	"github.com/lengzhao/vm/contractapi"
)

var embeddedContractAPISource = contractapi.Source
var embeddedContractAPIGasSource = contractapi.GasSource
var embeddedContractAPIProtocolSource = contractapi.ProtocolSource

const compilerBuildID = "pipeline-v11-artifact-manifest"

//go:embed entry.go.tmpl
var contractEntryTemplate string
