package builder

import networkingv1 "k8s.io/api/networking/v1"

// NetworkPolicyBuilder is a builder for networkingv1.NetworkPolicy objects to be used only in unit tests.
type NetworkPolicyBuilder struct {
	networkPolicy *networkingv1.NetworkPolicy
}

// NewNetworkPolicy returns a new NetworkPolicyBuilder. It is meant to be used only in unit tests.
func NewNetworkPolicy() *NetworkPolicyBuilder {
	return &NetworkPolicyBuilder{
		networkPolicy: &networkingv1.NetworkPolicy{},
	}
}

func (n *NetworkPolicyBuilder) WithName(name string) *NetworkPolicyBuilder {
	n.networkPolicy.Name = name
	return n
}

func (n *NetworkPolicyBuilder) WithNamespace(namespace string) *NetworkPolicyBuilder {
	n.networkPolicy.Namespace = namespace
	return n
}

func (n *NetworkPolicyBuilder) Build() *networkingv1.NetworkPolicy {
	return n.networkPolicy
}
