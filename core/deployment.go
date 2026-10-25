package core

const DeploymentVersion = 1
const CodeDeploymentConflict ErrorCode = "DEPLOYMENT_CONFLICT"

// DeploymentInput is explicit address-domain data, not authentication or nonce accounting.
type DeploymentInput struct {
	ChainID string   `json:"chain_id"`
	Sender  Address  `json:"sender"`
	Nonce   uint64   `json:"nonce"`
	Salt    [32]byte `json:"salt"`
}

type DeploymentRequest struct {
	Contract *CompiledContract
	Input    DeploymentInput
}

// DeploymentRecord binds a deployed address to its input and immutable artifact.
type DeploymentRecord struct {
	Version        int             `json:"version"`
	Input          DeploymentInput `json:"input"`
	ArtifactSHA256 string          `json:"artifact_sha256"`
}
