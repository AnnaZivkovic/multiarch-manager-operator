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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/openshift/multiarch-tuning-operator/pkg/utils"
)

const (
	healthPort   int32 = 8081
	metricsPort  int32 = 8443
	webhookPort  int32 = 9443
	dnsPort      int32 = 5353
	apiPort      int32 = 6443
	registryPort int32 = 443
)

// buildNetworkPolicyPodPlacement returns the additive NetworkPolicy for the
// Deployment-based operands (controller, webhook, and enoexec handler).
// Peers follow the OpenShift/OLM convention: openshift-dns on TCP/UDP 5353,
// destination-less TCP 6443 for the host-networked/HCP API server,
// openshift-monitoring on TCP 8443, destination-less TCP 9443 for admission,
// and destination-less TCP 443 for image inspection.
func buildNetworkPolicyPodPlacement() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PodPlacementNetworkPolicyName,
			Namespace: utils.Namespace(),
			Labels: map[string]string{
				utils.OperandLabelKey: operandName,
			},
			Annotations: map[string]string{
				"kubernetes.io/description": "Additive NetworkPolicy for Multiarch Tuning Operator pod-placement operands. DNS uses openshift-dns on TCP/UDP 5353. API egress is destination-less TCP 6443 because the API server is host-networked and HCP makes pod/ClusterIP selectors unreliable. Webhook ingress is destination-less TCP 9443 for the same reason. Registry egress is destination-less TCP 443 because image inspection contacts arbitrary registries.",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					utils.OperandLabelKey: operandName,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				healthIngressRule(),
				metricsIngressRule(),
				webhookIngressRule(),
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				dnsEgressRule(),
				apiEgressRule(),
				registryEgressRule(),
			},
		},
	}
}

// buildNetworkPolicyENoExecDaemon returns an egress-only NetworkPolicy for the
// ENoExec daemon. Incomplete egress-only policies are default-deny, so DNS 5353
// and API 6443 are both required. The daemon does not inspect images, so 443 is omitted.
func buildNetworkPolicyENoExecDaemon() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.EnoexecDaemonSet,
			Namespace: utils.Namespace(),
			Labels: map[string]string{
				"app": utils.EnoexecDaemonSet,
			},
			Annotations: map[string]string{
				"kubernetes.io/description": "Egress-only NetworkPolicy for the ENoExec daemon. DNS uses openshift-dns on TCP/UDP 5353. API egress is destination-less TCP 6443 because the API server is host-networked and HCP makes pod/ClusterIP selectors unreliable.",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": utils.EnoexecDaemonSet,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				dnsEgressRule(),
				apiEgressRule(),
			},
		},
	}
}

func healthIngressRule() networkingv1.NetworkPolicyIngressRule {
	return networkingv1.NetworkPolicyIngressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			networkPolicyPort(corev1.ProtocolTCP, healthPort),
		},
	}
}

func metricsIngressRule() networkingv1.NetworkPolicyIngressRule {
	return networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						utils.OpenShiftMonitoringNamespaceLabelKey: utils.OpenShiftMonitoringNamespace,
					},
				},
			},
		},
		Ports: []networkingv1.NetworkPolicyPort{
			networkPolicyPort(corev1.ProtocolTCP, metricsPort),
		},
	}
}

func webhookIngressRule() networkingv1.NetworkPolicyIngressRule {
	return networkingv1.NetworkPolicyIngressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			networkPolicyPort(corev1.ProtocolTCP, webhookPort),
		},
	}
}

func dnsEgressRule() networkingv1.NetworkPolicyEgressRule {
	return networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						utils.OpenShiftDNSNamespaceLabelKey: utils.OpenShiftDNSNamespaceName,
					},
				},
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						utils.OpenShiftDNSPodLabelKey: utils.OpenShiftDNSPodLabelValue,
					},
				},
			},
		},
		Ports: []networkingv1.NetworkPolicyPort{
			networkPolicyPort(corev1.ProtocolTCP, dnsPort),
			networkPolicyPort(corev1.ProtocolUDP, dnsPort),
		},
	}
}

func apiEgressRule() networkingv1.NetworkPolicyEgressRule {
	// Destination-less: the API server is host-networked and HCP callers are not guest pods.
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			networkPolicyPort(corev1.ProtocolTCP, apiPort),
		},
	}
}

func registryEgressRule() networkingv1.NetworkPolicyEgressRule {
	// Destination-less: image inspection contacts arbitrary registries.
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			networkPolicyPort(corev1.ProtocolTCP, registryPort),
		},
	}
}

func networkPolicyPort(protocol corev1.Protocol, port int32) networkingv1.NetworkPolicyPort {
	p := intstr.FromInt32(port)
	proto := protocol
	return networkingv1.NetworkPolicyPort{
		Protocol: &proto,
		Port:     &p,
	}
}
