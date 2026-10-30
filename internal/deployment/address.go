package deployment

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/lengzhao/vm/core"
)

// DeriveAddress implements deployment v1. It reads no clock, filesystem or host state.
func DeriveAddress(input core.DeploymentInput, manifestSHA256 string) (string, error) {
	for _, value := range []string{input.ChainID, string(input.Sender)} {
		if len(value) == 0 || len(value) > 256 || !utf8.ValidString(value) {
			return "", &core.ExecutionError{Code: core.CodeInvalidRequest, Message: "deployment chain ID and sender must each be 1–256 UTF-8 bytes"}
		}
	}
	digest, err := hex.DecodeString(manifestSHA256)
	if err != nil || len(digest) != sha256.Size || strings.ToLower(manifestSHA256) != manifestSHA256 {
		return "", &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "deployment requires a full lowercase artifact manifest SHA-256", Cause: err}
	}
	h := sha256.New()
	_, _ = h.Write([]byte("vm-deployment-v1\x00"))
	for _, value := range []string{input.ChainID, string(input.Sender)} {
		var length [4]byte
		binary.BigEndian.PutUint32(length[:], uint32(len(value)))
		_, _ = h.Write(length[:])
		_, _ = h.Write([]byte(value))
	}
	var nonce [8]byte
	binary.BigEndian.PutUint64(nonce[:], input.Nonce)
	_, _ = h.Write(nonce[:])
	_, _ = h.Write(input.Salt[:])
	_, _ = h.Write(digest)
	return "contract_" + hex.EncodeToString(h.Sum(nil)), nil
}

// ValidateRecord checks deployment consistency without changing compilation identity.
func ValidateRecord(c *core.CompiledContract) error {
	if c == nil || c.Deployment == nil {
		return nil
	}
	record := c.Deployment
	if record.Version != core.DeploymentVersion {
		return &core.ExecutionError{Code: core.CodeUnsupportedVersion, Message: "unsupported deployment version; redeploy"}
	}
	if record.ArtifactSHA256 != c.ManifestSHA256 {
		return &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "deployment artifact digest mismatch"}
	}
	address, err := DeriveAddress(record.Input, record.ArtifactSHA256)
	if err != nil {
		return err
	}
	if c.Address != address {
		return &core.ExecutionError{Code: core.CodeArtifactIntegrity, Message: "deployment address does not match its inputs"}
	}
	return nil
}
