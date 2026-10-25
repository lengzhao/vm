package policy

import (
	"context"
	"fmt"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact"
)

// ArtifactOptions separates accepted policy identities from required guarantees.
type ArtifactOptions struct {
	RequiredGuarantees core.PolicyGuarantees
	AcceptedPolicies   []string
	AllowAnyPolicy     bool
}
type ArtifactValidator struct {
	required core.PolicyGuarantees
	accepted map[string]bool
	allowAny bool
}

func NewArtifactValidator(o ArtifactOptions, source core.SourcePolicy) (*ArtifactValidator, error) {
	if source == nil || source.ID() == "" {
		return nil, fmt.Errorf("artifact source policy required")
	}
	required := o.RequiredGuarantees
	required.Deterministic = required.Deterministic || core.GuaranteesOf(source).Deterministic
	if o.AllowAnyPolicy && (required.Deterministic || source.ID() != developmentSourcePolicy) {
		return nil, fmt.Errorf("allowAnyPolicy is limited to the non-deterministic development policy")
	}
	ids := o.AcceptedPolicies
	if len(ids) == 0 {
		ids = []string{source.ID()}
	}
	accepted := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("accepted policy ID cannot be empty")
		}
		accepted[id] = true
	}
	return &ArtifactValidator{required: required, accepted: accepted, allowAny: o.AllowAnyPolicy}, nil
}
func (v *ArtifactValidator) ValidateArtifact(ctx context.Context, a *core.CompiledContract) error {
	if ctx == nil {
		return fmt.Errorf("artifact context required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil {
		return fmt.Errorf("contract cannot be nil")
	}
	if !v.allowAny && !v.accepted[a.SourcePolicy] {
		return fmt.Errorf("source policy mismatch: got %q", a.SourcePolicy)
	}
	if v.required.Deterministic && !a.Guarantees.Deterministic {
		return fmt.Errorf("artifact does not declare required deterministic guarantee")
	}
	if err := artifact.Validate(ctx, a); err != nil {
		return err
	}
	return artifact.ValidateNativeRules(a)
}
