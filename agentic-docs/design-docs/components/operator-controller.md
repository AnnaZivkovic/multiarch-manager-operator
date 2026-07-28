# Component: Operator Controller

**Location**: `internal/controller/operator/clusterpodplacementconfig_controller.go`  
**Mode**: `--enable-operator`  
**Purpose**: Manages ClusterPodPlacementConfig CR lifecycle and deploys/undeploys pod placement operands

## Responsibilities

1. **Reconcile ClusterPodPlacementConfig CR**: Watch singleton CR (name: `cluster`), deploy operands based on spec
2. **Deploy Operands**: Create deployments for pod placement controller and webhook
3. **Update Status Conditions**: Report Available, Progressing, Degraded, Deprovisioning states
4. **Ordered Deletion**: Ensure pods are ungated before operand removal to prevent orphaned gates

## Key Files

- `clusterpodplacementconfig_controller.go` (1016 lines): Main reconciliation logic
- `podplacement_objects.go`: Operand deployment manifests (controller, webhook, RBAC)
- `enoexecevent_objects.go`: ENoExecEvent daemon and handler manifests
- `objects.go`: Shared object builders (ServiceMonitor, Namespace)

## Reconciliation Logic

```go
func (r *ClusterPodPlacementConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch ClusterPodPlacementConfig CR
    // 2. If deletion timestamp set:
    //    - Remove operands (webhook first, then controller)
    //    - Wait for all gated pods to be ungated
    //    - Remove finalizer
    // 3. If no deletion:
    //    - Deploy/update pod placement controller deployment
    //    - Deploy/update pod placement webhook deployment
    //    - Create ServiceMonitor for metrics scraping
    //    - Update status conditions
    // 4. Requeue if operands not ready
}
```

## Status Conditions

| Condition | True When | False When |
|-----------|-----------|------------|
| **Available** | All operands running and ready | Any operand not ready |
| **Progressing** | Operands being deployed/updated | Operands stable |
| **Degraded** | Image inspection failures, webhook errors | No errors |
| **Deprovisioning** | CR deletion in progress | CR not being deleted |

## Deployment Strategy

**Controller Deployment**:
- Replicas: 1 (leader election ensures single active controller)
- Resources: CPU 10m-100m, Memory 64Mi-512Mi
- Image pull policy: IfNotPresent

**Webhook Deployment**:
- Replicas: 2 (stateless, can scale horizontally)
- Resources: CPU 10m-100m, Memory 32Mi-256Mi
- Liveness/Readiness probes: `/healthz` endpoint

## Operand Configuration

The operator passes configuration to operands via flags:

```go
// Pod placement controller flags
--enable-ppc-controllers
--enable-cppc-informer  // Enable CPPC singleton syncer
--global-pull-secret-namespace=openshift-config
--global-pull-secret-name=pull-secret

// Pod placement webhook flags
--enable-ppc-webhook
--enable-cppc-informer
--cert-dir=/etc/webhook/certs
```

## Deletion Flow

**Ordered Deletion** (critical for avoiding orphaned scheduling gates):

1. **Scale down webhook**: Prevent new gates from being added
2. **Wait for webhook termination**: Ensure no in-flight requests
3. **Ungate all pods**: Remove scheduling gates from all pending pods
4. **Scale down controller**: Stop processing gates
5. **Remove finalizer**: Allow CR deletion

**Ungating logic**:
```bash
kubectl get pods -A -l multiarch.openshift.io/scheduling-gate=gated -o json | \
  jq 'del(.items[].spec.schedulingGates[] | select(.name=="multiarch.openshift.io/scheduling-gate"))' | \
  kubectl apply -f -
```

## Metrics

The operator deploys a ServiceMonitor for Prometheus scraping:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
spec:
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
  namespaceSelector:
    matchNames:
      - openshift-multiarch-tuning-operator
```

## Leader Election

**Lease Name**: `clusterpodplacementconfig-operator-lock`  
**Lease Duration**: 137s  
**Renew Deadline**: 107s  
**Retry Period**: 26s

Only one operator controller instance active at a time. Leader election prevents split-brain scenarios.

## Related Components

- [→ Pod Placement Controller](pod-placement-controller.md) - Deployed operand
- [→ Pod Placement Webhook](pod-placement-webhook.md) - Deployed operand
- [→ CPPC Informer](../../domain/concepts/cppc-informer.md) - Singleton config syncer

## Related Concepts

- [→ ClusterPodPlacementConfig API](../../domain/concepts/clusterpodplacementconfig-api.md) - CR schema
- [→ Operator Pattern](../../domain/concepts/operator-pattern.md) - Operator-of-operators design
