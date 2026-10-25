// Package policy registers versioned source policies.
package policy

import (
	"github.com/lengzhao/pluginkit"
	"github.com/lengzhao/vm/core"
	impl "github.com/lengzhao/vm/internal/policy"
)

type Config struct {
	EnableSecurity bool `json:"enableSecurity"`
	Deterministic  bool `json:"deterministic"`
}
type Deps struct {
	Reviewer core.SecurityReviewer `json:"reviewer"`
}

func New(c Config, d Deps) (core.SourcePolicy, error) {
	return impl.NewSource(impl.Options{EnableSecurity: c.EnableSecurity, Deterministic: c.Deterministic}, d.Reviewer)
}
func init() { pluginkit.Register("policy/go", New) }

type ArtifactConfig struct {
	RequireDeterminism bool     `json:"requireDeterminism"`
	AllowAnyPolicy     bool     `json:"allowAnyPolicy"`
	AcceptedPolicies   []string `json:"acceptedPolicies"`
}
type ArtifactDeps struct {
	Policy core.SourcePolicy `json:"policy"`
}

func NewArtifact(c ArtifactConfig, d ArtifactDeps) (core.ArtifactValidator, error) {
	return impl.NewArtifactValidator(impl.ArtifactOptions{
		RequiredGuarantees: core.PolicyGuarantees{Deterministic: c.RequireDeterminism},
		AcceptedPolicies:   c.AcceptedPolicies, AllowAnyPolicy: c.AllowAnyPolicy,
	}, d.Policy)
}
func init() { pluginkit.Register("policy/artifact", NewArtifact) }
