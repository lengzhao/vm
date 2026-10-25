// Package deployment assigns contract addresses and applies deployment rules.
package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/lengzhao/vm/core"
	"os"
	"time"
)

type Deployer struct {
	writer    core.ContractWriter
	validator core.ArtifactValidator
}

func New(w core.ContractWriter, v core.ArtifactValidator) (*Deployer, error) {
	if w == nil || v == nil {
		return nil, fmt.Errorf("deployment writer and artifact validator required")
	}
	return &Deployer{writer: w, validator: v}, nil
}
func (d *Deployer) Deploy(ctx context.Context, c *core.CompiledContract) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if c == nil {
		return "", fmt.Errorf("contract cannot be nil")
	}
	if err := d.validator.ValidateArtifact(ctx, c); err != nil {
		return "", err
	}
	if c.ExecutablePath == "" {
		return "", fmt.Errorf("executable path cannot be empty")
	}
	if _, err := os.Stat(c.ExecutablePath); err != nil {
		return "", fmt.Errorf("executable unavailable: %w", err)
	}
	address := NewAddress(c.SourceHash)
	// The deployed address belongs to the stored record, not the compiled artifact.
	snapshot := *c
	snapshot.Address = address
	if err := d.writer.Put(ctx, address, &snapshot); err != nil {
		return "", fmt.Errorf("failed to store contract: %w", err)
	}
	return address, nil
}

// NewAddress is the native deployment address rule, shared by the legacy adapter.
func NewAddress(sourceHash string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s_%d", sourceHash, time.Now().UnixNano())))
	return "contract_" + hex.EncodeToString(hash[:])[:16]
}
