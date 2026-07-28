# Component: Pod Placement Webhook

**Location**: `internal/controller/podplacement/scheduling_gate_mutating_webhook.go`  
**Mode**: `--enable-ppc-webhook`  
**Purpose**: Mutating webhook that adds scheduling gates to new pods

## Responsibilities

1. **Intercept Pod Creations**: Mutating admission webhook for pod CREATE operations
2. **Add Scheduling Gate**: Inject `multiarch.openshift.io/scheduling-gate` to hold pods before scheduling
3. **Respect Namespace Selector**: Only gate pods in namespaces matching ClusterPodPlacementConfig selector
4. **Skip System Namespaces**: Never gate pods in `openshift-*`, `kube-*`, `hypershift-*` namespaces
5. **Publish Events**: Audit trail for gating decisions

## Key Files

- `scheduling_gate_mutating_webhook.go`: Webhook admission handler
- `events.go`: Event publishing (uses worker pool for async publishing)
- `metrics/webhook.go`: Prometheus metrics

## Admission Logic

```go
func (w *PodSchedulingGateMutatingWebHook) Handle(ctx context.Context, req admission.Request) admission.Response {
    // 1. Decode pod from admission request
    // 2. Check if pod already has scheduling gate → skip
    // 3. Check if namespace excluded (openshift-*, kube-*, hypershift-*) → skip
    // 4. Check if namespace matches selector from ClusterPodPlacementConfig → skip if not
    // 5. Add multiarch.openshift.io/scheduling-gate to pod.spec.schedulingGates
    // 6. Add label multiarch.openshift.io/scheduling-gate=gated to pod.metadata.labels
    // 7. Publish event (async via worker pool)
    // 8. Return patched pod
}
```

## MutatingWebhookConfiguration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: multiarch-tuning-operator-pod-placement-webhook
webhooks:
  - name: pod-placement-webhook.multiarch.openshift.io
    failurePolicy: Fail  # Fail-closed: reject pods if webhook down
    sideEffects: None
    admissionReviewVersions: ["v1"]
    timeoutSeconds: 10
    clientConfig:
      service:
        name: multiarch-tuning-operator-pod-placement-webhook
        namespace: openshift-multiarch-tuning-operator
        path: /mutate-v1-pod
      caBundle: <base64-encoded-CA>  # From cert-manager
    rules:
      - operations: ["CREATE"]
        apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
        scope: "Namespaced"
```

## Namespace Filtering

**Hard-coded Exclusions** (always skipped, cannot be overridden):
```go
func isNamespaceExcluded(namespace string) bool {
    return strings.HasPrefix(namespace, "openshift-") ||
           strings.HasPrefix(namespace, "kube-") ||
           strings.HasPrefix(namespace, "hypershift-")
}
```

**User-Defined Selector** (from ClusterPodPlacementConfig):
```go
// Example: Exclude namespaces with label multiarch.openshift.io/exclude-pod-placement
namespaceSelector := cppc.Spec.NamespaceSelector
if !namespaceSelector.Matches(namespace) {
    return admission.Allowed("namespace excluded by selector")
}
```

## Scheduling Gate Injection

**Before**:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
spec:
  containers:
    - name: nginx
      image: docker.io/library/nginx:latest
```

**After** (webhook mutation):
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  labels:
    multiarch.openshift.io/scheduling-gate: "gated"  # Added label
spec:
  schedulingGates:
    - name: "multiarch.openshift.io/scheduling-gate"  # Added gate
  containers:
    - name: nginx
      image: docker.io/library/nginx:latest
```

## Event Publishing

**Async Worker Pool**: Uses [ants library](https://github.com/panjf2000/ants) with 16 workers

**Rationale**: Event publishing to Kubernetes API is slow. Worker pool prevents webhook response latency from being blocked by event recording.

```go
// Submit event publishing to worker pool (non-blocking)
_ = w.eventPublisherPool.Submit(func() {
    w.recorder.Event(pod, corev1.EventTypeNormal, "SchedulingGateAdded", 
        "Pod placed in multi-arch scheduling queue")
})
```

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `mto_ppo_wh_pods_processed_total` | Counter | Total pods processed by webhook |
| `mto_ppo_wh_pods_gated_total` | Counter | Total pods gated |
| `mto_ppo_wh_response_time_seconds` | Histogram | Webhook response latency |

**Target P95 Response Time**: < 500ms

## TLS Certificate Management

**Certificate Authority**: Kubernetes CA (via cert-manager)

**Certificate Secret**: `multiarch-tuning-operator-pod-placement-webhook-cert`

**Rotation**: Automatic (cert-manager renews 30 days before expiration)

**Webhook Startup**: Waits for certificate to be available before starting server

## Failure Policy

**Fail-Closed** (`failurePolicy: Fail`): If webhook is down, pod creation requests are rejected.

**Rationale**: Allowing pods without scheduling gates risks placement on incompatible nodes (exec format errors).

**Mitigation**: Horizontal webhook scaling (2+ replicas) with readiness probes.

## Related Components

- [→ Pod Placement Controller](pod-placement-controller.md) - Processes gated pods
- [→ Operator Controller](operator-controller.md) - Deploys webhook
- [→ CPPC Informer](../../domain/concepts/cppc-informer.md) - Namespace selector source

## Related Concepts

- [→ Scheduling Gates](../../domain/concepts/scheduling-gates.md) - K8s scheduling gate mechanism
- [→ Admission Control](../../domain/concepts/admission-control.md) - Webhook admission flow
