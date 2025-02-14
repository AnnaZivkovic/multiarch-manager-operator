package builder

import (
	v1 "k8s.io/api/core/v1"
)

// PreferredSchedulingTermBuilder is a builder for v1.NodeSelectorTerm objects to be used only in unit tests.
type PreferredSchedulingTermBuilder struct {
	preferredSchedulingTerm *v1.PreferredSchedulingTerm
}

// NewPreferredSchedulingTerm returns a new PreferredSchedulingTermBuilder to build v1.NodeSelectorTerm objects. It is meant to be used only in unit tests.
func NewPreferredSchedulingTerm() *PreferredSchedulingTermBuilder {
	return &PreferredSchedulingTermBuilder{
		preferredSchedulingTerm: &v1.PreferredSchedulingTerm{},
	}
}

func (p *PreferredSchedulingTermBuilder) WithKeyAndValues(weight int, nodeSelectorTerm v1.NodeSelectorTerm) *PreferredSchedulingTermBuilder {
	p.preferredSchedulingTerm.Weight = int32(weight)
	p.preferredSchedulingTerm.Preference = nodeSelectorTerm
	return p
}

func (p *PreferredSchedulingTermBuilder) Build() *v1.PreferredSchedulingTerm {
	return p.preferredSchedulingTerm
}
