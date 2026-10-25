// Package deployment assigns deterministic contract addresses and applies deployment rules.
package deployment

import (
	"context"
	"fmt"
	"github.com/lengzhao/vm/core"
)

type Deployer struct {
	creator   core.ContractCreator
	validator core.ArtifactValidator
}

func New(c core.ContractCreator, v core.ArtifactValidator) (*Deployer, error) {
	if c == nil || v == nil {
		return nil, fmt.Errorf("deployment creator and artifact validator required")
	}
	return &Deployer{creator: c, validator: v}, nil
}
func (d *Deployer) Deploy(ctx context.Context, req core.DeploymentRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if req.Contract == nil {
		return "", fmt.Errorf("contract cannot be nil")
	}
	// Validate the producer's artifact before deriving a new deployed snapshot.
	if err := d.validator.ValidateArtifact(ctx, req.Contract); err != nil {
		return "", err
	}
	address, err := DeriveAddress(req.Input, req.Contract.ManifestSHA256)
	if err != nil {
		return "", err
	}
	snapshot := *req.Contract
	snapshot.Address = address
	snapshot.Deployment = &core.DeploymentRecord{Version: core.DeploymentVersion, Input: req.Input, ArtifactSHA256: snapshot.ManifestSHA256}
	if err := d.validator.ValidateArtifact(ctx, &snapshot); err != nil {
		return "", err
	}
	if err := d.creator.Create(ctx, address, &snapshot); err != nil {
		return "", fmt.Errorf("failed to create contract: %w", err)
	}
	return address, nil
}
