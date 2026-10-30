package core

// These versions describe runtime semantics, independently of source policy IDs.
const ArtifactManifestVersion = 1
const ArtifactRuntimeVersion = "contractapi-v3-call"
const ArtifactGasVersion = "go-gas-v3-call"
const CodeArtifactIntegrity ErrorCode = "ARTIFACT_INTEGRITY"

// ArtifactToolchain records the actual compiler and effective build target.
type ArtifactToolchain struct {
	ContainerImage string   `json:"container_image,omitempty"`
	Version        string   `json:"version"`
	BinarySHA256   string   `json:"binary_sha256"`
	GOOS           string   `json:"goos"`
	GOARCH         string   `json:"goarch"`
	CGOEnabled     bool     `json:"cgo_enabled"`
	Flags          []string `json:"flags"`
}

// ArtifactBuild identifies the inputs that produced an executable. Digests are
// lowercase, full-length SHA-256. CompilerVersion identifies a pipeline, not a guarantee.
type ArtifactBuild struct {
	CompilerVersion string            `json:"compiler_version"`
	Toolchain       ArtifactToolchain `json:"toolchain"`
	RuntimeVersion  string            `json:"runtime_version"`
	GasVersion      string            `json:"gas_version"`
	RuntimeSHA256   string            `json:"runtime_sha256"`
	SourceSHA256    string            `json:"source_sha256"`
	InjectedSHA256  string            `json:"injected_sha256"`
	EntrySHA256     string            `json:"entry_sha256"`
}

// ArtifactManifest binds an executable to its ABI, gas profile and policy
// metadata. Integrity is not authentication of the producer or the source.
type ArtifactManifest struct {
	Version          int           `json:"version"`
	Build            ArtifactBuild `json:"build"`
	ProtocolVersion  int           `json:"protocol_version"`
	ABIVersion       int           `json:"abi_version"`
	BinarySHA256     string        `json:"binary_sha256"`
	ABISHA256        string        `json:"abi_sha256"`
	GasProfileSHA256 string        `json:"gas_profile_sha256"`
	MetadataSHA256   string        `json:"metadata_sha256"`
}
