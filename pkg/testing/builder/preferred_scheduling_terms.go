package builder

import (
	v1 "k8s.io/api/core/v1"
)

// PreferredSchedulingTermsBuilder is a builder for v1.NodeSelectorTerm objects to be used only in unit tests.
type PreferredSchedulingTermsBuilder struct {
	preferredSchedulingTerms []v1.PreferredSchedulingTerm
}

// NewPreferredSchedulingTerms returns a new PreferredSchedulingTermsBuilder to build v1.NodeSelectorTerm objects. It is meant to be used only in unit tests.
func NewPreferredSchedulingTerms() *PreferredSchedulingTermsBuilder {
	return &PreferredSchedulingTermsBuilder{
		preferredSchedulingTerms: []v1.PreferredSchedulingTerm{},
	}
}

func (p *PreferredSchedulingTermsBuilder) WithPreferredSchedulingTerm(preferredSchedulingTerm *v1.PreferredSchedulingTerm) *PreferredSchedulingTermsBuilder {
	p.preferredSchedulingTerms = append(p.preferredSchedulingTerms, *preferredSchedulingTerm)
	return p
}

func (p *PreferredSchedulingTermsBuilder) WithArchitectureWeight(architecture string, weight int32) *PreferredSchedulingTermsBuilder {
	preferredSchedulingTerm := NewPreferredSchedulingTerm().WithArchitecture(architecture).WithWeight(weight).Build()
	p.preferredSchedulingTerms = append(p.preferredSchedulingTerms, *preferredSchedulingTerm)
	return p
}

func (p *PreferredSchedulingTermsBuilder) Build() []v1.PreferredSchedulingTerm {
	return p.preferredSchedulingTerms
}
