// Package artifact implements content integrity, without deployment or policy decisions.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/contractapi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/deployment"
)

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func JSONDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return Digest(data), nil
}

func RuntimeDigest() string {
	digest, _ := JSONDigest([]string{contractapi.Source, contractapi.GasSource, contractapi.ProtocolSource, contractapi.ObjectSource, contractapi.CallSource})
	return digest
}

// FileDigest streams a regular executable, observing cancellation between reads.
func FileDigest(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("artifact must be a regular executable")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return "", fmt.Errorf("executable changed while opening")
	}
	h := sha256.New()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := f.Read(buffer)
		if n > 0 {
			_, _ = h.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func metadataDigest(a *core.CompiledContract) (string, error) {
	return JSONDigest(struct {
		SourceHash      string                `json:"source_hash"`
		SourcePolicy    string                `json:"source_policy"`
		Guarantees      core.PolicyGuarantees `json:"guarantees"`
		ProtocolVersion int                   `json:"protocol_version"`
	}{a.SourceHash, a.SourcePolicy, a.Guarantees, a.ProtocolVersion})
}

// Seal is called only by a trusted producer; consumers must never re-seal inputs.
func Seal(ctx context.Context, a *core.CompiledContract, build core.ArtifactBuild) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil {
		return integrity("contract required", nil)
	}
	if a.ABI == nil {
		return integrity("ABI required", nil)
	}
	binary, err := FileDigest(ctx, a.ExecutablePath)
	if err != nil {
		return integrity("read executable", err)
	}
	abiDigest, err := JSONDigest(a.ABI)
	if err != nil {
		return err
	}
	profile, err := JSONDigest(a.GasProfile)
	if err != nil {
		return err
	}
	metadata, err := metadataDigest(a)
	if err != nil {
		return err
	}
	a.Manifest = &core.ArtifactManifest{Version: core.ArtifactManifestVersion, Build: build, ProtocolVersion: a.ProtocolVersion, ABIVersion: a.ABI.Version, BinarySHA256: binary, ABISHA256: abiDigest, GasProfileSHA256: profile, MetadataSHA256: metadata}
	a.ManifestSHA256, err = JSONDigest(a.Manifest)
	if err != nil {
		return err
	}
	return Validate(ctx, a)
}

func integrity(message string, cause error) error {
	return &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: message, Cause: cause}
}
func unsupported(message string) error {
	return &core.ExecutionError{Code: core.CodeUnsupportedVersion, Message: message + "; recompile and deploy"}
}

// Validate checks the representation and content, leaving accepted source
// policies and supported execution semantics to the runtime's validator.
func Validate(ctx context.Context, a *core.CompiledContract) error {
	if ctx == nil {
		return integrity("artifact context required", nil)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil {
		return integrity("contract required", nil)
	}
	if a.ProtocolVersion != core.ProtocolVersion {
		return unsupported("unsupported artifact protocol")
	}
	if err := abi.ValidateCallable(a.ABI); err != nil {
		return &core.ExecutionError{Code: core.CodeUnsupportedABI, Message: err.Error(), Cause: err}
	}
	m := a.Manifest
	if m == nil || m.Version != core.ArtifactManifestVersion {
		return unsupported("missing or unsupported artifact manifest")
	}
	if m.ProtocolVersion != a.ProtocolVersion || m.ABIVersion != a.ABI.Version {
		return unsupported("artifact schema version mismatch")
	}
	b := m.Build
	if b.CompilerVersion == "" || b.Toolchain.Version == "" || b.Toolchain.GOOS == "" || b.Toolchain.GOARCH == "" || b.RuntimeVersion == "" || b.GasVersion == "" {
		return unsupported("incomplete artifact build rules")
	}
	for _, digest := range []string{b.SourceSHA256, b.InjectedSHA256, b.EntrySHA256, b.RuntimeSHA256, b.Toolchain.BinarySHA256, m.BinarySHA256, m.ABISHA256, m.GasProfileSHA256, m.MetadataSHA256, a.ManifestSHA256} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != sha256.Size || strings.ToLower(digest) != digest {
			return integrity("invalid SHA-256 in manifest", err)
		}
	}
	manifestDigest, err := JSONDigest(m)
	if err != nil {
		return err
	}
	if manifestDigest != a.ManifestSHA256 {
		return integrity("artifact manifest digest mismatch", nil)
	}
	abiDigest, err := JSONDigest(a.ABI)
	if err != nil {
		return err
	}
	profile, err := JSONDigest(a.GasProfile)
	if err != nil {
		return err
	}
	metadata, err := metadataDigest(a)
	if err != nil {
		return err
	}
	if abiDigest != m.ABISHA256 || profile != m.GasProfileSHA256 || metadata != m.MetadataSHA256 {
		return integrity("artifact metadata digest mismatch", nil)
	}
	binary, err := FileDigest(ctx, a.ExecutablePath)
	if err != nil {
		return integrity("read artifact executable", err)
	}
	if binary != m.BinarySHA256 {
		return integrity("artifact binary digest mismatch", nil)
	}
	if err := deployment.ValidateRecord(a); err != nil {
		return err
	}
	return ctx.Err()
}

// ValidateNativeRules is the compatibility check for the built-in native runtime.
func ValidateNativeRules(a *core.CompiledContract) error {
	if a == nil || a.Manifest == nil {
		return unsupported("artifact manifest required")
	}
	b := a.Manifest.Build
	if b.RuntimeVersion != core.ArtifactRuntimeVersion || b.GasVersion != core.ArtifactGasVersion || b.RuntimeSHA256 != RuntimeDigest() {
		return unsupported("unsupported runtime or gas rules")
	}
	if b.Toolchain.GOOS != runtime.GOOS || b.Toolchain.GOARCH != runtime.GOARCH || b.Toolchain.CGOEnabled {
		return unsupported("unsupported native artifact target")
	}
	if a.Address != "" && a.Deployment == nil {
		return unsupported("deployment inputs required for deployed artifact")
	}
	return nil
}
