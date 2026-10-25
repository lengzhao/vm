package contractapi

import _ "embed"

// Source is the contract-side API embedded in standalone contract builds.
//
//go:embed contractapi.go
var Source string

//go:embed gas.go
var GasSource string

//go:embed protocol.go
var ProtocolSource string

//go:embed object.go
var ObjectSource string

//go:embed call.go
var CallSource string
