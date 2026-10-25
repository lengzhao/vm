// Package cap exports the VM extension points. Implementations should depend on
// cap/core, never the root compatibility package or another plugin.
package cap

import "github.com/lengzhao/vm/core"

type Engine = core.Engine
type Compiler = core.ContractCompiler
type ABI = core.ABIGenerator
type SecurityReviewer = core.SecurityReviewer
type SourcePolicy = core.SourcePolicy
type ContractReader = core.ContractReader
type ContractWriter = core.ContractWriter
type ContractStore = core.ContractStore
type Deployer = core.ContractDeployer
type ArtifactValidator = core.ArtifactValidator
type Executor = core.Executor
type Runner = core.Runner
type GasInjector = core.GasInjector
type GasProfileBuilder = core.GasProfileBuilder
type GasEstimator = core.GasEstimator
type GasProfiler = core.GasProfiler
type GasMeterFactory = core.GasMeterFactory
type GasMeter = core.GasMetering
