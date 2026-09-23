/*
Copyright 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package operator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

const registryPort int32 = 443
const healthPort int32 = 8081 // kubelet host-network probes; must not appear as a NetworkPolicy rule

func TestBuildNetworkPolicyPodPlacement(t *testing.T) {
	np := buildNetworkPolicyPodPlacement()
	if np.Name != utils.PodPlacementNetworkPolicyName {
		t.Fatalf("name: got %q", np.Name)
	}
	assertOperatorNamespace(t, np)
	if got := np.Spec.PodSelector.MatchLabels[utils.OperandLabelKey]; got != operandName {
		t.Fatalf("podSelector: got %q", got)
	}
	if _, ok := np.Spec.PodSelector.MatchLabels[utils.ControllerNameKey]; ok {
		t.Fatal("shared operand policy must not require the controller label")
	}
	assertPolicyTypes(t, np, networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress)
	assertNoIPBlock(t, np)
	assertNoIngressPort(t, np, healthPort)
	assertIngressPortFromMonitoring(t, np, metricsPort)
	assertIngressPort(t, np, webhookPort, true)
	assertDNSEgress(t, np)
	assertDestinationLessEgressPort(t, np, apiPort)
	if hasDestinationLessEgressPort(np, registryPort) {
		t.Fatal("shared operand policy must not allow registry TCP 443")
	}
	if hasDestinationLessTCPAllPorts(np) {
		t.Fatal("shared operand policy must not allow destination-less TCP on all ports")
	}
}

func TestBuildNetworkPolicyPodPlacementImageInspection(t *testing.T) {
	np := buildNetworkPolicyPodPlacementImageInspection()
	if np.Name != utils.PodPlacementImageInspectionNetworkPolicyName {
		t.Fatalf("name: got %q", np.Name)
	}
	assertOperatorNamespace(t, np)
	if got := np.Spec.PodSelector.MatchLabels[utils.ControllerNameKey]; got != utils.PodPlacementControllerName {
		t.Fatalf("controller selector: got %q", got)
	}
	if got := np.Spec.PodSelector.MatchLabels[utils.OperandLabelKey]; got != operandName {
		t.Fatalf("operand selector: got %q", got)
	}
	assertPolicyTypes(t, np, networkingv1.PolicyTypeEgress)
	if len(np.Spec.Ingress) != 0 {
		t.Fatalf("image-inspection policy must be egress-only, got %d ingress rules", len(np.Spec.Ingress))
	}
	assertNoIPBlock(t, np)
	if !hasDestinationLessTCPAllPorts(np) {
		t.Fatal("image-inspection policy must allow destination-less TCP on all ports")
	}
}

func TestBuildNetworkPolicyENoExecDaemon(t *testing.T) {
	np := buildNetworkPolicyENoExecDaemon()
	if np.Name != utils.EnoexecDaemonSet {
		t.Fatalf("name: got %q", np.Name)
	}
	assertOperatorNamespace(t, np)
	if got := np.Spec.PodSelector.MatchLabels["app"]; got != utils.EnoexecDaemonSet {
		t.Fatalf("podSelector: got %q", got)
	}
	assertPolicyTypes(t, np, networkingv1.PolicyTypeEgress)
	if len(np.Spec.Ingress) != 0 {
		t.Fatalf("daemon policy must be egress-only, got %d ingress rules", len(np.Spec.Ingress))
	}
	assertNoIPBlock(t, np)
	assertDNSEgress(t, np)
	assertDestinationLessEgressPort(t, np, apiPort)
	if hasDestinationLessEgressPort(np, registryPort) {
		t.Fatal("daemon policy must not allow registry TCP 443")
	}
	if hasDestinationLessTCPAllPorts(np) {
		t.Fatal("daemon policy must not allow destination-less TCP on all ports")
	}
}

func TestBuildNetworkPolicyManager(t *testing.T) {
	np := buildNetworkPolicyManager()
	if np.Name != utils.ManagerNetworkPolicyName {
		t.Fatalf("name: got %q", np.Name)
	}
	assertOperatorNamespace(t, np)
	if got := np.Labels["control-plane"]; got != "controller-manager" {
		t.Fatalf("labels: got %q", np.Labels)
	}
	if got := np.Spec.PodSelector.MatchLabels["control-plane"]; got != "controller-manager" {
		t.Fatalf("podSelector: got %q", got)
	}
	assertPolicyTypes(t, np, networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress)
	assertNoIPBlock(t, np)
	assertNoIngressPort(t, np, healthPort)
	assertIngressPort(t, np, webhookPort, true)
	assertIngressPortFromMonitoring(t, np, metricsPort)
	assertDNSEgress(t, np)
	assertDestinationLessEgressPort(t, np, apiPort)
	if hasDestinationLessTCPAllPorts(np) {
		t.Fatal("manager policy must not allow destination-less TCP on all ports")
	}
}

func TestDefaultKustomizationOmitsNetwork(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "default", "kustomization.yaml"))
	if err != nil {
		t.Fatalf("read kustomization: %v", err)
	}
	if strings.Contains(string(data), "../network") {
		t.Fatal("config/default/kustomization.yaml must not include ../network; the manager NetworkPolicy is created at runtime")
	}
}

func TestDropControllerOwnerReferences(t *testing.T) {
	controlled := true
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{Name: "old-deploy", UID: "uid-a", Controller: &controlled},
				{Name: "keep-me", UID: "uid-cm"},
			},
		},
	}
	dropControllerOwnerReferences(np)
	if len(np.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences len: got %d", len(np.OwnerReferences))
	}
	if np.OwnerReferences[0].Name != "keep-me" {
		t.Fatalf("non-controller ownerReference was dropped: got %q", np.OwnerReferences[0].Name)
	}
}

func TestNetworkPoliciesAreCreatedInOperatorNamespace(t *testing.T) {
	policies := []*networkingv1.NetworkPolicy{
		buildNetworkPolicyPodPlacement(),
		buildNetworkPolicyPodPlacementImageInspection(),
		buildNetworkPolicyENoExecDaemon(),
		buildNetworkPolicyManager(),
	}
	for _, np := range policies {
		assertOperatorNamespace(t, np)
	}
}

func TestNamespacedOperandRBACIsNotClusterScoped(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "rbac", "role.yaml"))
	if err != nil {
		t.Fatalf("read role.yaml: %v", err)
	}
	clusterRole, namespacedRole, ok := splitManagerRoles(string(data))
	if !ok {
		t.Fatal("config/rbac/role.yaml must contain both a ClusterRole and a namespaced Role")
	}
	for _, resource := range []string{"networkpolicies", "deployments", "daemonsets", "servicemonitors"} {
		if strings.Contains(clusterRole, "- "+resource) {
			t.Errorf("ClusterRole must not grant %s; bind it on the namespaced Role", resource)
		}
		if !strings.Contains(namespacedRole, "- "+resource) {
			t.Errorf("namespaced Role must grant %s", resource)
		}
	}
}

func splitManagerRoles(raw string) (clusterRole, namespacedRole string, ok bool) {
	for _, doc := range strings.Split(raw, "\n---\n") {
		switch {
		case strings.Contains(doc, "\nkind: ClusterRole\n"):
			clusterRole = doc
		case strings.Contains(doc, "\nkind: Role\n"):
			namespacedRole = doc
		}
	}
	return clusterRole, namespacedRole, clusterRole != "" && namespacedRole != ""
}

func assertOperatorNamespace(t *testing.T, np *networkingv1.NetworkPolicy) {
	t.Helper()
	if np.Namespace != utils.Namespace() {
		t.Fatalf("%s namespace: got %q want %q", np.Name, np.Namespace, utils.Namespace())
	}
}

func assertPolicyTypes(t *testing.T, np *networkingv1.NetworkPolicy, want ...networkingv1.PolicyType) {
	t.Helper()
	if len(np.Spec.PolicyTypes) != len(want) {
		t.Fatalf("policyTypes len: got %v want %v", np.Spec.PolicyTypes, want)
	}
	for i := range want {
		if np.Spec.PolicyTypes[i] != want[i] {
			t.Fatalf("policyTypes[%d]: got %s want %s", i, np.Spec.PolicyTypes[i], want[i])
		}
	}
}

func assertNoIPBlock(t *testing.T, np *networkingv1.NetworkPolicy) {
	t.Helper()
	for _, rule := range np.Spec.Egress {
		for _, peer := range rule.To {
			if peer.IPBlock != nil {
				t.Fatalf("unexpected egress ipBlock: %+v", peer.IPBlock)
			}
		}
	}
	for _, rule := range np.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.IPBlock != nil {
				t.Fatalf("unexpected ingress ipBlock: %+v", peer.IPBlock)
			}
		}
	}
}

func assertDNSEgress(t *testing.T, np *networkingv1.NetworkPolicy) {
	t.Helper()
	for _, rule := range np.Spec.Egress {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, dnsPort) || !hasPort(rule.Ports, corev1.ProtocolUDP, dnsPort) {
			continue
		}
		if len(rule.To) != 1 {
			t.Fatalf("dns egress should have one peer, got %d", len(rule.To))
		}
		peer := rule.To[0]
		if peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels[utils.OpenShiftDNSNamespaceLabelKey] != utils.OpenShiftDNSNamespaceName {
			t.Fatalf("dns namespaceSelector: %+v", peer.NamespaceSelector)
		}
		if peer.PodSelector != nil {
			t.Fatalf("dns egress must be namespace-only, got podSelector %+v", peer.PodSelector)
		}
		return
	}
	t.Fatal("missing DNS egress to openshift-dns on TCP/UDP 5353")
}

func assertDestinationLessEgressPort(t *testing.T, np *networkingv1.NetworkPolicy, port int32) {
	t.Helper()
	if !hasDestinationLessEgressPort(np, port) {
		t.Fatalf("missing destination-less egress TCP %d", port)
	}
}

func hasDestinationLessEgressPort(np *networkingv1.NetworkPolicy, port int32) bool {
	for _, rule := range np.Spec.Egress {
		if len(rule.To) != 0 {
			continue
		}
		if hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			return true
		}
	}
	return false
}

func hasDestinationLessTCPAllPorts(np *networkingv1.NetworkPolicy) bool {
	for _, rule := range np.Spec.Egress {
		if len(rule.To) != 0 {
			continue
		}
		if len(rule.Ports) == 0 {
			return true
		}
		for _, p := range rule.Ports {
			if p.Protocol != nil && *p.Protocol == corev1.ProtocolTCP && p.Port == nil {
				return true
			}
		}
	}
	return false
}

func assertIngressPort(t *testing.T, np *networkingv1.NetworkPolicy, port int32, openFrom bool) {
	t.Helper()
	for _, rule := range np.Spec.Ingress {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		if openFrom && len(rule.From) != 0 {
			t.Fatalf("port %d should have no From peers, got %+v", port, rule.From)
		}
		return
	}
	t.Fatalf("missing ingress TCP %d", port)
}

func assertNoIngressPort(t *testing.T, np *networkingv1.NetworkPolicy, port int32) {
	t.Helper()
	for _, rule := range np.Spec.Ingress {
		if hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			t.Fatalf("ingress TCP %d must not be listed; kubelet probes are host-networked", port)
		}
	}
}

func assertIngressPortFromMonitoring(t *testing.T, np *networkingv1.NetworkPolicy, port int32) {
	t.Helper()
	for _, rule := range np.Spec.Ingress {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		if len(rule.From) != 1 {
			t.Fatalf("metrics ingress should have one peer, got %d", len(rule.From))
		}
		peer := rule.From[0]
		if peer.NamespaceSelector == nil || peer.NamespaceSelector.MatchLabels[utils.OpenShiftMonitoringNamespaceLabelKey] != utils.OpenShiftMonitoringNamespace {
			t.Fatalf("metrics namespaceSelector: %+v", peer.NamespaceSelector)
		}
		return
	}
	t.Fatal("missing metrics ingress from openshift-monitoring")
}

func hasPort(ports []networkingv1.NetworkPolicyPort, protocol corev1.Protocol, port int32) bool {
	want := intstr.FromInt32(port)
	for _, p := range ports {
		if p.Port == nil || p.Protocol == nil {
			continue
		}
		if *p.Protocol == protocol && *p.Port == want {
			return true
		}
	}
	return false
}
