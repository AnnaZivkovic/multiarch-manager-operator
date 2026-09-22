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

package framework

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

const (
	networkPolicyHealthPort  int32 = 8081
	networkPolicyMetricsPort int32 = 8443
	networkPolicyWebhookPort int32 = 9443
	networkPolicyDNSPort     int32 = 5353
	networkPolicyAPIPort     int32 = 6443
)

// VerifyOperandNetworkPolicies asserts the ClusterPodPlacementConfig operand
// NetworkPolicies exist and match the OpenShift/OLM contract.
func VerifyOperandNetworkPolicies(ctx context.Context, c client.Client) func(gomega.Gomega) {
	return func(g gomega.Gomega) {
		ginkgo.By("Verify the shared operand NetworkPolicy")
		operands := &networkingv1.NetworkPolicy{}
		err := c.Get(ctx, client.ObjectKey{
			Name:      utils.PodPlacementNetworkPolicyName,
			Namespace: utils.Namespace(),
		}, operands)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get operand NetworkPolicy")
		assertSharedOperandNetworkPolicy(g, operands)

		ginkgo.By("Verify the image-inspection NetworkPolicy")
		inspection := &networkingv1.NetworkPolicy{}
		err = c.Get(ctx, client.ObjectKey{
			Name:      utils.PodPlacementImageInspectionNetworkPolicyName,
			Namespace: utils.Namespace(),
		}, inspection)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get image-inspection NetworkPolicy")
		assertImageInspectionNetworkPolicy(g, inspection)
	}
}

// VerifyENoExecDaemonNetworkPolicy asserts the ENoExec daemon NetworkPolicy.
func VerifyENoExecDaemonNetworkPolicy(ctx context.Context, c client.Client) func(gomega.Gomega) {
	return func(g gomega.Gomega) {
		ginkgo.By("Verify the ENoExec daemon NetworkPolicy")
		np := &networkingv1.NetworkPolicy{}
		err := c.Get(ctx, client.ObjectKey{
			Name:      utils.EnoexecDaemonSet,
			Namespace: utils.Namespace(),
		}, np)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get ENoExec daemon NetworkPolicy")
		assertENoExecDaemonNetworkPolicy(g, np)
	}
}

// VerifyManagerNetworkPolicy asserts the OLM/Kustomize manager NetworkPolicy.
func VerifyManagerNetworkPolicy(ctx context.Context, c client.Client) func(gomega.Gomega) {
	return func(g gomega.Gomega) {
		ginkgo.By("Verify the manager NetworkPolicy")
		np := &networkingv1.NetworkPolicy{}
		err := c.Get(ctx, client.ObjectKey{
			Name:      utils.ManagerNetworkPolicyName,
			Namespace: utils.Namespace(),
		}, np)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get manager NetworkPolicy")
		assertManagerNetworkPolicy(g, np)
	}
}

func assertSharedOperandNetworkPolicy(g gomega.Gomega, np *networkingv1.NetworkPolicy) {
	g.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.Equal(map[string]string{
		utils.OperandLabelKey: utils.PodPlacementControllerName,
	}))
	g.Expect(np.Spec.PodSelector.MatchLabels).NotTo(gomega.HaveKey(utils.ControllerNameKey))
	assertPolicyTypes(g, np, networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress)
	assertNoIPBlock(g, np)
	assertOpenIngressPort(g, np, networkPolicyHealthPort)
	assertMetricsIngressFromMonitoring(g, np)
	assertOpenIngressPort(g, np, networkPolicyWebhookPort)
	assertDNSEgress(g, np)
	assertDestinationLessEgressPort(g, np, networkPolicyAPIPort)
	g.Expect(hasDestinationLessTCPAllPorts(np)).To(gomega.BeFalse(),
		"shared operand policy must not allow destination-less TCP on all ports")
}

func assertImageInspectionNetworkPolicy(g gomega.Gomega, np *networkingv1.NetworkPolicy) {
	g.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.Equal(map[string]string{
		utils.OperandLabelKey:   utils.PodPlacementControllerName,
		utils.ControllerNameKey: utils.PodPlacementControllerName,
	}))
	assertPolicyTypes(g, np, networkingv1.PolicyTypeEgress)
	g.Expect(np.Spec.Ingress).To(gomega.BeEmpty(), "image-inspection policy must be egress-only")
	assertNoIPBlock(g, np)
	g.Expect(hasDestinationLessTCPAllPorts(np)).To(gomega.BeTrue(),
		"image-inspection policy must allow destination-less TCP on all ports")
}

func assertENoExecDaemonNetworkPolicy(g gomega.Gomega, np *networkingv1.NetworkPolicy) {
	g.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.Equal(map[string]string{
		"app": utils.EnoexecDaemonSet,
	}))
	assertPolicyTypes(g, np, networkingv1.PolicyTypeEgress)
	g.Expect(np.Spec.Ingress).To(gomega.BeEmpty(), "daemon policy must be egress-only")
	assertNoIPBlock(g, np)
	assertDNSEgress(g, np)
	assertDestinationLessEgressPort(g, np, networkPolicyAPIPort)
	g.Expect(hasDestinationLessTCPAllPorts(np)).To(gomega.BeFalse(),
		"daemon policy must not allow destination-less TCP on all ports")
}

func assertManagerNetworkPolicy(g gomega.Gomega, np *networkingv1.NetworkPolicy) {
	g.Expect(np.Spec.PodSelector.MatchLabels).To(gomega.Equal(map[string]string{
		"control-plane": "controller-manager",
	}))
	assertPolicyTypes(g, np, networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress)
	assertNoIPBlock(g, np)
	assertOpenIngressPort(g, np, networkPolicyHealthPort)
	assertOpenIngressPort(g, np, networkPolicyWebhookPort)
	assertMetricsIngressFromMonitoring(g, np)
	assertDNSEgress(g, np)
	assertDestinationLessEgressPort(g, np, networkPolicyAPIPort)
	g.Expect(hasDestinationLessTCPAllPorts(np)).To(gomega.BeFalse(),
		"manager policy must not allow destination-less TCP on all ports")
	for key := range np.Annotations {
		g.Expect(key).NotTo(gomega.HavePrefix("include.release.openshift.io/"),
			"manager policy must not include CVO payload annotation %s", key)
	}
}

func assertPolicyTypes(g gomega.Gomega, np *networkingv1.NetworkPolicy, want ...networkingv1.PolicyType) {
	g.Expect(np.Spec.PolicyTypes).To(gomega.Equal(want))
}

func assertNoIPBlock(g gomega.Gomega, np *networkingv1.NetworkPolicy) {
	for _, rule := range np.Spec.Egress {
		for _, peer := range rule.To {
			g.Expect(peer.IPBlock).To(gomega.BeNil(), "unexpected egress ipBlock")
		}
	}
	for _, rule := range np.Spec.Ingress {
		for _, peer := range rule.From {
			g.Expect(peer.IPBlock).To(gomega.BeNil(), "unexpected ingress ipBlock")
		}
	}
}

func assertDNSEgress(g gomega.Gomega, np *networkingv1.NetworkPolicy) {
	for _, rule := range np.Spec.Egress {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, networkPolicyDNSPort) || !hasPort(rule.Ports, corev1.ProtocolUDP, networkPolicyDNSPort) {
			continue
		}
		g.Expect(rule.To).To(gomega.HaveLen(1), "dns egress should have one peer")
		peer := rule.To[0]
		g.Expect(peer.NamespaceSelector).NotTo(gomega.BeNil())
		g.Expect(peer.NamespaceSelector.MatchLabels).To(gomega.HaveKeyWithValue(
			utils.OpenShiftDNSNamespaceLabelKey, utils.OpenShiftDNSNamespaceName))
		g.Expect(peer.PodSelector).To(gomega.BeNil(), "dns egress must be namespace-only")
		return
	}
	g.Expect(true).To(gomega.BeFalse(), "missing DNS egress to openshift-dns on TCP/UDP 5353")
}

func assertDestinationLessEgressPort(g gomega.Gomega, np *networkingv1.NetworkPolicy, port int32) {
	g.Expect(hasDestinationLessEgressPort(np, port)).To(gomega.BeTrue(),
		"missing destination-less egress TCP %d", port)
}

func assertOpenIngressPort(g gomega.Gomega, np *networkingv1.NetworkPolicy, port int32) {
	for _, rule := range np.Spec.Ingress {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, port) {
			continue
		}
		g.Expect(rule.From).To(gomega.BeEmpty(), "port %d should have no From peers", port)
		return
	}
	g.Expect(true).To(gomega.BeFalse(), "missing ingress TCP %d", port)
}

func assertMetricsIngressFromMonitoring(g gomega.Gomega, np *networkingv1.NetworkPolicy) {
	for _, rule := range np.Spec.Ingress {
		if !hasPort(rule.Ports, corev1.ProtocolTCP, networkPolicyMetricsPort) {
			continue
		}
		g.Expect(rule.From).To(gomega.HaveLen(1), "metrics ingress should have one peer")
		peer := rule.From[0]
		g.Expect(peer.NamespaceSelector).NotTo(gomega.BeNil())
		g.Expect(peer.NamespaceSelector.MatchLabels).To(gomega.HaveKeyWithValue(
			utils.OpenShiftMonitoringNamespaceLabelKey, utils.OpenShiftMonitoringNamespace))
		return
	}
	g.Expect(true).To(gomega.BeFalse(), "missing metrics ingress from openshift-monitoring")
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
