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
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

func TestBuildNetworkPolicyPodPlacement(t *testing.T) {
	np := buildNetworkPolicyPodPlacement()
	if np.Name != utils.PodPlacementNetworkPolicyName {
		t.Fatalf("name: got %q", np.Name)
	}
	if got := np.Spec.PodSelector.MatchLabels[utils.OperandLabelKey]; got != operandName {
		t.Fatalf("podSelector: got %q", got)
	}
	assertPolicyTypes(t, np, networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress)
	assertNoIPBlock(t, np)
	assertIngressPort(t, np, healthPort, true)
	assertIngressPortFromMonitoring(t, np, metricsPort)
	assertIngressPort(t, np, webhookPort, true)
	assertDNSEgress(t, np)
	assertDestinationLessEgressPort(t, np, apiPort)
	assertDestinationLessEgressPort(t, np, registryPort)
}

func TestBuildNetworkPolicyENoExecDaemon(t *testing.T) {
	np := buildNetworkPolicyENoExecDaemon()
	if np.Name != utils.EnoexecDaemonSet {
		t.Fatalf("name: got %q", np.Name)
	}
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
}

func TestManagerNetworkPolicyYAML(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "network", "controller-manager-policy.yaml"))
	if err != nil {
		t.Fatalf("read yaml: %v", err)
	}
	if strings.Contains(string(data), "include.release.openshift.io") {
		t.Fatal("manager policy must not include CVO HyperShift payload annotations")
	}
	np := &networkingv1.NetworkPolicy{}
	if err := yaml.Unmarshal(data, np); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if np.Name != "controller-manager" {
		t.Fatalf("name: got %q", np.Name)
	}
	if got := np.Spec.PodSelector.MatchLabels["control-plane"]; got != "controller-manager" {
		t.Fatalf("podSelector: got %q", got)
	}
	assertPolicyTypes(t, np, networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress)
	assertNoIPBlock(t, np)
	assertIngressPort(t, np, healthPort, true)
	assertIngressPortFromMonitoring(t, np, metricsPort)
	assertDNSEgress(t, np)
	assertDestinationLessEgressPort(t, np, apiPort)
	if hasDestinationLessEgressPort(np, registryPort) {
		t.Fatal("manager policy must not allow registry TCP 443")
	}
	if hasIngressPort(np, webhookPort) {
		t.Fatal("manager policy must not open webhook 9443")
	}
}

func TestDefaultKustomizationIncludesNetwork(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "default", "kustomization.yaml"))
	if err != nil {
		t.Fatalf("read kustomization: %v", err)
	}
	if !strings.Contains(string(data), "../network") {
		t.Fatal("config/default/kustomization.yaml must include ../network")
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
		if peer.PodSelector == nil || peer.PodSelector.MatchLabels[utils.OpenShiftDNSPodLabelKey] != utils.OpenShiftDNSPodLabelValue {
			t.Fatalf("dns podSelector: %+v", peer.PodSelector)
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
