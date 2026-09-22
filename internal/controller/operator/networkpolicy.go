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
	healthPort  int32 = 8081
	metricsPort int32 = 8443
	webhookPort int32 = 9443
	dnsPort     int32 = 5353
	apiPort     int32 = 6443
)

// buildNetworkPolicyPodPlacement returns the additive NetworkPolicy for the
// Deployment-based operands (controller, webhook, and enoexec handler).
// Peers follow the OpenShift/OLM convention: openshift-dns on TCP/UDP 5353,
// destination-less TCP 6443 for the host-networked/HCP API server,
// openshift-monitoring on TCP 8443, and destination-less TCP 9443 for admission.
// Registry egress is not granted here; only the image-inspection controller
// needs it (see buildNetworkPolicyPodPlacementImageInspection).
func buildNetworkPolicyPodPlacement() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PodPlacementNetworkPolicyName,
			Namespace: utils.Namespace(),
			Labels: map[string]string{
				utils.OperandLabelKey: operandName,
			},
			Annotations: map[string]string{
				"kubernetes.io/description": "Additive NetworkPolicy for Multiarch Tuning Operator pod-placement operands. DNS uses the openshift-dns namespace on TCP/UDP 5353. API egress is destination-less TCP 6443 because the API server is host-networked and HCP makes pod/ClusterIP selectors unreliable. Webhook ingress is destination-less TCP 9443 for the same reason. Registry egress is granted only by the image-inspection policy on the pod-placement controller.",
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
			},
		},
	}
}

// buildNetworkPolicyPodPlacementImageInspection returns an egress-only policy
// for the pod-placement controller, which is the only operand that inspects
// container images. Destination-less TCP with no port restriction is required
// so inspection can reach any registry a pod image might use (443, OpenShift
// integrated registry 5000, insecure 80, mirrors, and cluster proxies).
func buildNetworkPolicyPodPlacementImageInspection() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.PodPlacementImageInspectionNetworkPolicyName,
			Namespace: utils.Namespace(),
			Labels: map[string]string{
				utils.OperandLabelKey:   operandName,
				utils.ControllerNameKey: utils.PodPlacementControllerName,
			},
			Annotations: map[string]string{
				"kubernetes.io/description": "Egress-only NetworkPolicy for pod-placement-controller image inspection. Destination-less TCP with no port covers arbitrary registries and cluster proxies so any image a user schedules can be inspected.",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					utils.OperandLabelKey:   operandName,
					utils.ControllerNameKey: utils.PodPlacementControllerName,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				imageInspectionEgressRule(),
			},
		},
	}
}

// buildNetworkPolicyENoExecDaemon returns an egress-only NetworkPolicy for the
// ENoExec daemon. Incomplete egress-only policies are default-deny, so DNS 5353
// and API 6443 are both required. The daemon does not inspect images, so
// registry egress is omitted.
func buildNetworkPolicyENoExecDaemon() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.EnoexecDaemonSet,
			Namespace: utils.Namespace(),
			Labels: map[string]string{
				"app": utils.EnoexecDaemonSet,
			},
			Annotations: map[string]string{
				"kubernetes.io/description": "Egress-only NetworkPolicy for the ENoExec daemon. DNS uses the openshift-dns namespace on TCP/UDP 5353. API egress is destination-less TCP 6443 because the API server is host-networked and HCP makes pod/ClusterIP selectors unreliable.",
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

// buildNetworkPolicyManager returns the additive NetworkPolicy for the
// operator manager Deployment. Peers follow the same OpenShift/OLM
// convention as the operand policies. Image-inspection egress is omitted
// because the manager does not inspect container images.
func buildNetworkPolicyManager() *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.ManagerNetworkPolicyName,
			Namespace: utils.Namespace(),
			Labels: map[string]string{
				"control-plane": "controller-manager",
			},
			Annotations: map[string]string{
				"kubernetes.io/description": "Additive NetworkPolicy for the Multiarch Tuning Operator manager. DNS uses the openshift-dns namespace on TCP/UDP 5353. API egress is destination-less TCP 6443 because the API server is host-networked and HCP makes pod/ClusterIP selectors unreliable. Webhook ingress is destination-less TCP 9443 for conversion and validating admission on the manager.",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"control-plane": "controller-manager",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				healthIngressRule(),
				webhookIngressRule(),
				metricsIngressRule(),
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
	// Namespace-only: cluster-olm-operator dropped the daemonset-dns pod label
	// because it is not guaranteed on every supported OpenShift/HCP DNS topology.
	// AND-ing that label with PolicyTypeEgress would black-hole DNS if it is absent.
	return networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						utils.OpenShiftDNSNamespaceLabelKey: utils.OpenShiftDNSNamespaceName,
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

func imageInspectionEgressRule() networkingv1.NetworkPolicyEgressRule {
	// Destination-less TCP, all ports: users can pull from any registry host:port
	// (HTTPS 443, OpenShift image-registry 5000, insecure 80, custom mirrors, proxies).
	proto := corev1.ProtocolTCP
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &proto},
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
