# Manager NetworkPolicy

The Multiarch Tuning Operator ships additive `networking.k8s.io/v1` NetworkPolicies
for OpenShift (including HCP guest clusters). This is not a generic Kubernetes
networking API. Configurable/strict egress belongs in a separate non-OpenShift effort.

MTO is OLM-installed. Do not add CVO payload annotations such as
`include.release.openshift.io/hypershift`. Those only affect CVO-rendered
payloads and are a no-op in an OLM CSV.

## Split

| Workload | How it is created | Selector | Ingress | Egress |
| --- | --- | --- | --- | --- |
| Manager | Static YAML in this directory (Kustomize/OLM) | `control-plane: controller-manager` | TCP 8081 (open), TCP 8443 from `openshift-monitoring`, destination-less TCP 9443 | TCP/UDP 5353 to `openshift-dns` namespace, destination-less TCP 6443 |
| Pod-placement operands | Runtime Go (`buildNetworkPolicyPodPlacement`) | `multiarch.openshift.io/operand: pod-placement-controller` | TCP 8081 (open), TCP 8443 from monitoring, destination-less TCP 9443 | DNS 5353, API 6443 |
| Pod-placement controller (image inspection) | Runtime Go (`buildNetworkPolicyPodPlacementImageInspection`) | operand label **and** `controller: pod-placement-controller` | none (egress-only) | destination-less TCP, all ports |
| ENoExec daemon | Runtime Go (`buildNetworkPolicyENoExecDaemon`) | `app: enoexec-event-daemon` | none (egress-only) | DNS 5353, API 6443 |

## Why these ports

- **5353**: OpenShift CoreDNS listens on 5353, not Service port 53. NetworkPolicy is evaluated against the backend port. The peer is the `openshift-dns` namespace only; do not AND `dns.operator.openshift.io/daemonset-dns=default` (that label is not guaranteed on every supported OpenShift/HCP DNS topology, and with `PolicyTypeEgress` a miss black-holes DNS).
- **6443**: destination-less because the API server is host-networked and HCP admission/API paths are not guest pods. Do not use `kubernetes.default` ClusterIP as an `ipBlock`.
- **8443**: platform Prometheus in `openshift-monitoring`.
- **9443**: destination-less webhook ingress for the same HCP/host-network reason as 6443. Required on the manager (conversion and validating webhooks) and on the pod-placement webhook operand.
- **Image inspection TCP (all ports)**: destination-less TCP with no port, only on the pod-placement controller. Inspection must reach whatever registry a user image uses (HTTPS 443, OpenShift integrated registry `:5000`, insecure `:80`, custom mirrors, and cluster proxies). Pinning 443 is not enough. The manager, webhook, ENoExec handler, and daemon omit this hole.

Destination-less 6443, 9443, and all-TCP inspection egress are intentional compatibility holes, not failed peer discovery.

## Semantics

These policies are additive. There is no namespace-wide default-deny.
If a policy includes `policyTypes: [Egress]`, the listed holes are required.
An incomplete egress-only daemon policy is default-deny and must not be created.
Multiple policies that select the same pod union their allowed traffic.

## Troubleshooting

```bash
oc get networkpolicies -n openshift-multiarch-tuning-operator
oc describe networkpolicy -n openshift-multiarch-tuning-operator
```

Check operand logs for API/DNS connection errors if pods are not Ready.
Check pod-placement-controller logs if image inspection fails for a registry
on a non-443 port.
