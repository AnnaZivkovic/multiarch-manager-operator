# Concept: Scheduling Gates

## Overview

Scheduling gates are a Kubernetes mechanism to hold pods in Pending state before the scheduler can place them. The Multiarch Tuning Operator uses gates to synchronize pod creation with architecture analysis.

## Kubernetes API

**Introduced**: Kubernetes 1.27 (KEP-3521: Pod Scheduling Readiness)

**Pod Spec Field**:
```yaml
spec:
  schedulingGates:
    - name: "multiarch.openshift.io/scheduling-gate"
```

**Behavior**: Pods with non-empty `schedulingGates` are not considered for scheduling until all gates are removed.

## Operator Workflow

### 1. Webhook Adds Gate

**When**: Pod CREATE operation (admission webhook)

**What**: Inject `multiarch.openshift.io/scheduling-gate` into `pod.spec.schedulingGates`

**Why**: Prevent immediate scheduling before architecture analysis

### 2. Controller Processes Gate

**When**: PodReconciler detects pod with gate

**What**:
- Inspect container images
- Compute supported architectures
- Set nodeAffinity

### 3. Controller Removes Gate

**When**: Architecture analysis complete, nodeAffinity set

**What**: Delete gate from `pod.spec.schedulingGates`

**Effect**: Pod becomes eligible for scheduling

## Why Gates Instead of nodeAffinity-Only?

**Alternative**: Set nodeAffinity directly in webhook

**Problem**: Webhooks have timeout constraints (10s). Image inspection can take longer (registry latency).

**Solution**: Gate allows asynchronous processing:
- Webhook: Fast operation (add gate) → < 500ms response time
- Controller: Slow operation (inspect images) → can take seconds

## Gate Lifecycle

```
Pod Created
    ↓
[Webhook] Add scheduling gate
    ↓
Pod Pending (gated)
    ↓
[Controller] Inspect images
    ↓
[Controller] Set nodeAffinity
    ↓
[Controller] Remove scheduling gate
    ↓
Pod Pending (ungated, ready for scheduling)
    ↓
[Scheduler] Place pod on node
    ↓
Pod Running
```

## Orphaned Gates

**Scenario**: Pod placement controller crashes after adding gate but before removing it

**Impact**: Pod remains gated indefinitely

**Detection**: Metric `mto_ppo_pods_gated` should converge to 0. Sustained high value indicates orphans.

**Recovery**:
```bash
# Manual ungating
kubectl get pods -A -l multiarch.openshift.io/scheduling-gate=gated -o json | \
  jq 'del(.items[].spec.schedulingGates[] | select(.name=="multiarch.openshift.io/scheduling-gate"))' | \
  kubectl apply -f -
```

**Prevention**: Controller uses field selector to watch only `status.phase=Pending` pods with gates, reducing reconciliation failures.

## Related Documents

- [→ Component: Pod Placement Webhook](../../design-docs/components/pod-placement-webhook.md) - Adds gates
- [→ Component: Pod Placement Controller](../../design-docs/components/pod-placement-controller.md) - Removes gates
- [→ KEP-3521: Pod Scheduling Readiness](https://github.com/kubernetes/enhancements/tree/master/keps/sig-scheduling/3521-pod-scheduling-readiness)
