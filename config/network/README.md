# Manager NetworkPolicy

The Multiarch Tuning Operator ships additive `networking.k8s.io/v1` NetworkPolicies
for OpenShift (including HCP). This is not a generic Kubernetes networking API.
Configurable/strict egress belongs in a separate non-OpenShift effort.

## Split

| Workload | How it is created | Selector | Ingress | Egress |
| --- | --- | --- | --- | --- |
| Manager | Static YAML in this directory (Kustomize/OLM) | `control-plane: controller-manager` | TCP 8081 (open), TCP 8443 from `openshift-monitoring`, destination-less TCP 9443 | TCP/UDP 5353 to `openshift-dns`, destination-less TCP 6443 |
| Pod-placement operands | Runtime Go (`buildNetworkPolicyPodPlacement`) | `multiarch.openshift.io/operand: pod-placement-controller` | TCP 8081 (open), TCP 8443 from monitoring, destination-less TCP 9443 | DNS 5353, API 6443, destination-less TCP 443 |
| ENoExec daemon | Runtime Go (`buildNetworkPolicyENoExecDaemon`) | `app: enoexec-event-daemon` | none (egress-only) | DNS 5353, API 6443 |

## Why these ports

- **5353**: OpenShift CoreDNS listens on 5353, not Service port 53. NetworkPolicy is evaluated against the backend port.
- **6443**: destination-less because the API server is host-networked and HCP admission/API paths are not guest pods. Do not use `kubernetes.default` ClusterIP as an `ipBlock`.
- **8443**: platform Prometheus in `openshift-monitoring`.
- **9443**: destination-less webhook ingress for the same HCP/host-network reason as 6443. Required on the manager (conversion and validating webhooks) and on the pod-placement webhook operand.
- **443**: destination-less registry egress only on workloads that inspect images (pod-placement operands). The manager and daemon omit 443.

Destination-less 6443, 9443, and 443 are intentional compatibility holes, not failed peer discovery.

## Semantics

These policies are additive. There is no namespace-wide default-deny.
If a policy includes `policyTypes: [Egress]`, the listed holes are required.
An incomplete egress-only daemon policy is default-deny and must not be created.

## Troubleshooting

```bash
oc get networkpolicies -n openshift-multiarch-tuning-operator
oc describe networkpolicy -n openshift-multiarch-tuning-operator
```

Check operand logs for API/DNS connection errors if pods are not Ready.
