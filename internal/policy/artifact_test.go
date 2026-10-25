package policy

import (
	"context"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/artifact/artifacttest"
	"testing"
)

type customPolicy struct{}

func (customPolicy) ID() string          { return "custom-v2" }
func (customPolicy) Review(string) error { return nil }
func (customPolicy) Guarantees() core.PolicyGuarantees {
	return core.PolicyGuarantees{Deterministic: true}
}
func TestArtifactGuarantees(t *testing.T) {
	v, err := NewArtifactValidator(ArtifactOptions{}, customPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	a := &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourcePolicy: "custom-v2"}
	if v.ValidateArtifact(context.Background(), a) == nil {
		t.Fatal("missing guarantee accepted")
	}
	a.Guarantees.Deterministic = true
	artifacttest.Seal(t, a)
	if err := v.ValidateArtifact(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	a.SourcePolicy = "other"
	if v.ValidateArtifact(context.Background(), a) == nil {
		t.Fatal("foreign policy accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if v.ValidateArtifact(ctx, a) != context.Canceled {
		t.Fatal("cancellation ignored")
	}
	if v.ValidateArtifact(context.Background(), nil) == nil {
		t.Fatal("nil accepted")
	}
}

type plainPolicy struct{}

func (plainPolicy) ID() string          { return "custom-deterministic-looking-name" }
func (plainPolicy) Review(string) error { return nil }
func TestArtifactOptions(t *testing.T) {
	t.Run("identity is not a guarantee", func(t *testing.T) {
		v, err := NewArtifactValidator(ArtifactOptions{}, plainPolicy{})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.ValidateArtifact(context.Background(), artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourcePolicy: plainPolicy{}.ID()})); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("explicit requirement", func(t *testing.T) {
		v, err := NewArtifactValidator(ArtifactOptions{RequiredGuarantees: core.PolicyGuarantees{Deterministic: true}}, plainPolicy{})
		if err != nil {
			t.Fatal(err)
		}
		if v.ValidateArtifact(context.Background(), artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourcePolicy: plainPolicy{}.ID()})) == nil {
			t.Fatal("missing guarantee accepted")
		}
	})
	t.Run("allow any cannot bypass deterministic policy", func(t *testing.T) {
		if _, err := NewArtifactValidator(ArtifactOptions{AllowAnyPolicy: true}, customPolicy{}); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	})
	t.Run("development compatibility is explicit", func(t *testing.T) {
		source, err := NewSource(Options{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		v, err := NewArtifactValidator(ArtifactOptions{AllowAnyPolicy: true}, source)
		if err != nil {
			t.Fatal(err)
		}
		if err := v.ValidateArtifact(context.Background(), artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourcePolicy: "legacy"})); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("explicit accepted policies", func(t *testing.T) {
		v, err := NewArtifactValidator(ArtifactOptions{AcceptedPolicies: []string{"custom-v3"}}, customPolicy{})
		if err != nil {
			t.Fatal(err)
		}
		if err := v.ValidateArtifact(context.Background(), artifacttest.Seal(t, &core.CompiledContract{ProtocolVersion: core.ProtocolVersion, SourcePolicy: "custom-v3", Guarantees: core.PolicyGuarantees{Deterministic: true}})); err != nil {
			t.Fatal(err)
		}
	})
}
