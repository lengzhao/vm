package policy

import (
	"fmt"
	"github.com/lengzhao/vm/core"
	"github.com/lengzhao/vm/internal/security"
	"go/parser"
	"go/token"
)

// Source implements the existing development/deterministic source profiles.
type Source struct {
	Reviewer       core.SecurityReviewer
	EnableSecurity bool
	Deterministic  bool
}

func (p *Source) ID() string {
	if p.Deterministic {
		return DeterministicPolicyVersion
	}
	return developmentSourcePolicy
}
func (p *Source) Review(source string) error {
	if p.Deterministic {
		return reviewDeterministic(source, p.Reviewer)
	}
	if p.EnableSecurity {
		return p.Reviewer.Review(source)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "", source, parser.AllErrors)
	if err != nil {
		return err
	}
	return security.ReviewRuntimeBoundary(file)
}

const developmentSourcePolicy = "go-development-v3-call"

type Options struct {
	EnableSecurity bool
	Deterministic  bool
}

func NewSource(o Options, reviewer core.SecurityReviewer) (*Source, error) {
	if reviewer == nil && (o.EnableSecurity || o.Deterministic) {
		return nil, fmt.Errorf("source policy reviewer required")
	}
	return &Source{Reviewer: reviewer, EnableSecurity: o.EnableSecurity, Deterministic: o.Deterministic}, nil
}
func (p *Source) Guarantees() core.PolicyGuarantees {
	return core.PolicyGuarantees{Deterministic: p.Deterministic}
}
