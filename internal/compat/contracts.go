package compat

import (
	"context"
	"fmt"
	"github.com/lengzhao/vm/abi"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/deployment"
)

type ContractManager interface {
	Deploy(*core.CompiledContract) (string, error)
	GetContract(string) (*core.CompiledContract, error)
	GetContractABI(string) (*abi.ABI, error)
	GetGasProfile(string) (*core.GasProfile, error)
	StoreContract(*core.CompiledContract, string) error
	LoadContract(string) (*core.CompiledContract, error)
}
type Manager struct {
	store    core.ContractStore
	deployer core.ContractDeployer
}

func NewManager(s core.ContractStore, d core.ContractDeployer) *Manager {
	return &Manager{store: s, deployer: d}
}
func (m *Manager) GetContract(a string) (*core.CompiledContract, error) {
	return m.store.Get(context.Background(), a)
}
func (m *Manager) LoadContract(a string) (*core.CompiledContract, error) { return m.GetContract(a) }
func (m *Manager) GetContractABI(a string) (*abi.ABI, error) {
	c, err := m.GetContract(a)
	if err != nil {
		return nil, err
	}
	return c.ABI, nil
}
func (m *Manager) GetGasProfile(a string) (*core.GasProfile, error) {
	c, err := m.GetContract(a)
	if err != nil {
		return nil, err
	}
	if c.GasProfile == nil {
		return nil, fmt.Errorf("gas profile not found for contract %s", a)
	}
	return c.GasProfile, nil
}
func (m *Manager) StoreContract(c *core.CompiledContract, a string) error {
	if err := m.store.Put(context.Background(), a, c); err != nil {
		return err
	}
	loaded, err := m.GetContract(a)
	if err != nil {
		return err
	}
	c.ExecutablePath = loaded.ExecutablePath
	return nil
}
func (m *Manager) Deploy(c *core.CompiledContract) (string, error) {
	var a string
	var err error
	// The original raw manager also accepted metadata-only test artifacts. Keep
	// that behavior in the adapter; the executable deployer remains strict.
	if c != nil && c.ExecutablePath == "" {
		a = deployment.NewAddress(c.SourceHash)
		err = m.store.Put(context.Background(), a, c)
	} else {
		a, err = m.deployer.Deploy(context.Background(), c)
	}
	if err != nil {
		return "", err
	}
	loaded, err := m.GetContract(a)
	if err != nil {
		return "", err
	}
	c.Address = a
	c.ExecutablePath = loaded.ExecutablePath
	return a, nil
}

type AcceptArtifacts struct{}

func (AcceptArtifacts) ValidateArtifact(ctx context.Context, _ *core.CompiledContract) error {
	return ctx.Err()
}
