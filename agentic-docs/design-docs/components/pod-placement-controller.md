# Component: Pod Placement Controller

**Location**: `internal/controller/podplacement/pod_reconciler.go`  
**Mode**: `--enable-ppc-controllers`  
**Purpose**: Reconciles pods with scheduling gates, inspects images, injects nodeAffinity, removes gates

## Responsibilities

1. **Watch Gated Pods**: Monitor pods in Pending state with `multiarch.openshift.io/scheduling-gate` scheduling gate
2. **Inspect Container Images**: Determine supported architectures via registry manifest inspection
3. **Compute NodeAffinity**: Set required and preferred nodeAffinity based on image architectures
4. **Remove Scheduling Gate**: Ungate pod to allow scheduler to place it on compatible nodes
5. **Publish Events**: Audit trail for pod placement decisions

## Key Files

- `pod_reconciler.go`: Main reconciliation loop (watches gated pods)
- `pod_model.go`: Core business logic (image inspection, nodeAffinity computation, ungating)
- `global_pull_secret.go`: Syncs global pull secret for image authentication
- `events.go`: Event publishing for audit trail
- `metrics/controller.go`: Prometheus metrics

## Reconciliation Logic

```go
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch pod
    // 2. Check if pod has scheduling gate
    // 3. Verify pod should be processed (namespace selector, existing nodeAffinity)
    // 4. Retrieve image pull secrets
    // 5. Inspect all container images (parallel)
    // 6. Compute architecture set (union of all image architectures)
    // 7. Set required nodeAffinity (must-have constraint)
    // 8. Set preferred nodeAffinity (soft hint, if plugin enabled)
    // 9. Remove scheduling gate
    // 10. Publish event
}
```

## Concurrency Model

**MaxConcurrentReconciles**: `NumCPU * 4`

**Rationale**: Image inspection is I/O bound (network calls to registries). High concurrency maximizes throughput while waiting for registry responses.

**Field Selector Optimization**: Only watch `status.phase=Pending` pods to reduce reconciliation load.

## Image Inspection

**Flow**:
1. Extract image references from pod spec (init containers, containers, ephemeral containers)
2. For each image:
   - Check cache (in-memory `sync.Map`)
   - If cache miss: Call `pkg/image/Inspector.GetCompatibleArchitecturesSet()`
   - Aggregate architectures into set
3. If any inspection fails:
   - Retry with exponential backoff (max 5 attempts)
   - If `fallbackArchitecture` configured: Use fallback
   - Else: Leave pod gated, publish failure event

**Pull Secret Resolution**:
```go
// 1. Pod's imagePullSecrets
// 2. ServiceAccount's imagePullSecrets
// 3. Global pull secret (openshift-config/pull-secret)
```

## NodeAffinity Injection

**Required Affinity** (hard constraint):
```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: kubernetes.io/arch
                operator: In
                values: ["amd64", "arm64"]  # From image inspection
```

**Preferred Affinity** (soft hint, if NodeAffinityScoring plugin enabled):
```yaml
preferredDuringSchedulingIgnoredDuringExecution:
  - weight: 100
    preference:
      matchExpressions:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]  # Preferred architecture based on node distribution
```

## Max Retries Mechanism

If image inspection fails repeatedly (e.g., registry unreachable):
- Retry up to 5 times with exponential backoff
- After max retries:
  - If `fallbackArchitecture` configured: Use fallback, ungate pod
  - Else: Leave pod gated indefinitely (manual intervention required)

**Event published**: `ImageInspectionFailed` with details on failure reason

## Namespace Selector

Pods are processed only if their namespace matches `ClusterPodPlacementConfig.spec.namespaceSelector`.

**Hard-coded exclusions** (always skipped):
- `openshift-*`
- `kube-*`
- `hypershift-*`

## Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `mto_ppo_ctrl_time_to_process_pod_seconds` | Histogram | Total pod processing time |
| `mto_ppo_ctrl_time_to_process_gated_pod_seconds` | Histogram | Gated pod processing time (includes inspection) |
| `mto_ppo_ctrl_time_to_inspect_image_seconds` | Histogram | Single image inspection time |
| `mto_ppo_ctrl_processed_pods_total` | Counter | Total gated pods processed |
| `mto_ppo_ctrl_failed_image_inspection_total` | Counter | Failed inspections |

## Leader Election

**Lease Name**: `clusterpodplacementconfig-pod-placement-controllers-lock`

Only one controller instance actively reconciles at a time.

## Related Components

- [→ Pod Placement Webhook](pod-placement-webhook.md) - Adds scheduling gates
- [→ Image Inspector](image-inspector.md) - Registry manifest inspection
- [→ Operator Controller](operator-controller.md) - Deploys this controller

## Related Concepts

- [→ Scheduling Gates](../../domain/concepts/scheduling-gates.md) - Gating mechanism
- [→ Image Inspection](../../domain/concepts/image-inspection.md) - Architecture detection
- [→ NodeAffinity](../../domain/concepts/node-affinity.md) - Scheduler constraints
