# Workflow: Pod Placement Flow

## Overview

End-to-end flow from pod creation to scheduling on compatible architecture nodes.

## Actors

- **User/Controller**: Creates pod
- **API Server**: Receives pod creation request
- **Mutating Webhook**: Intercepts pod creation, adds scheduling gate
- **Pod Placement Controller**: Processes gated pod, inspects images, sets nodeAffinity
- **Scheduler**: Places ungated pod on compatible node
- **Kubelet**: Runs pod on node

## Flow Diagram

```
┌─────────┐
│  User   │
└────┬────┘
     │ kubectl create pod
     v
┌────────────────┐
│  API Server    │
└────┬───────────┘
     │ Admission webhook chain
     v
┌────────────────────────────┐
│  Mutating Webhook          │
│  (Pod Placement Webhook)   │
└────┬───────────────────────┘
     │ Add scheduling gate
     │ Add label
     v
┌────────────────┐
│  Pod (Gated)   │◄───┐
│  status:       │    │
│   phase: Pending    │
│  spec:              │
│   schedulingGates:  │
│    - name: multiarch│
└────┬───────────────┘│
     │ Watch (field   │
     │ selector)      │
     v                │
┌────────────────────────────┐
│  Pod Placement Controller  │
└────┬───────────────────────┘
     │ 1. Inspect images
     │ 2. Compute architectures
     │ 3. Set nodeAffinity
     │ 4. Remove gate
     │
     └──────────────────────────┘
                               │
┌────────────────┐            │
│  Pod (Ungated) │◄───────────┘
│  status:       │
│   phase: Pending
│  spec:
│   affinity:
│     nodeAffinity: {...}
└────┬───────────┘
     │ Scheduler
     │ scores nodes
     v
┌────────────────┐
│  Node (amd64)  │
│  Pod: Running  │
└────────────────┘
```

## Step-by-Step Flow

### 1. User Creates Pod

```bash
kubectl create -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  namespace: default
spec:
  containers:
    - name: nginx
      image: docker.io/library/nginx:latest
EOF
```

### 2. Webhook Intercepts Request

**Webhook Logic**:
- Check namespace: `default` → not in excluded list → proceed
- Check namespace selector: matches → proceed
- Add scheduling gate: `multiarch.openshift.io/scheduling-gate`
- Add label: `multiarch.openshift.io/scheduling-gate=gated`

**Mutated Pod**:
```yaml
spec:
  schedulingGates:
    - name: "multiarch.openshift.io/scheduling-gate"
  containers:
    - name: nginx
      image: docker.io/library/nginx:latest
metadata:
  labels:
    multiarch.openshift.io/scheduling-gate: "gated"
```

### 3. Pod Created (Gated)

**Status**: `Pending` (cannot be scheduled due to gate)

**Event**: `SchedulingGateAdded` event published

### 4. Controller Watches Pod

**Field Selector**: `status.phase=Pending,metadata.labels.multiarch.openshift.io/scheduling-gate=gated`

**Reconciliation Triggered**: Pod appears in watch stream

### 5. Controller Inspects Images

**Image**: `docker.io/library/nginx:latest`

**Inspection**:
1. Fetch manifest from registry
2. Parse manifest list
3. Extract architectures: `{amd64, arm64, ppc64le, s390x}`

**Cache**: Store result for future reconciliations

### 6. Controller Sets NodeAffinity

**Computed nodeAffinity**:
```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: kubernetes.io/arch
                operator: In
                values: ["amd64", "arm64", "ppc64le", "s390x"]
```

**Update**: Patch pod with nodeAffinity

### 7. Controller Removes Gate

**Update**: Delete `multiarch.openshift.io/scheduling-gate` from `spec.schedulingGates`

**Event**: `SchedulingGateRemoved` event published

### 8. Pod Becomes Eligible for Scheduling

**Status**: `Pending` (no gate, ready for scheduler)

### 9. Scheduler Places Pod

**Scheduling**:
- Score all nodes
- Filter by nodeAffinity (must match `kubernetes.io/arch`)
- Select node with highest score
- Bind pod to node

**Example**: Node `worker-1` (amd64) selected

### 10. Kubelet Runs Pod

**Kubelet**:
- Pull image: `docker.io/library/nginx:latest` (amd64 variant)
- Create container
- Start pod

**Status**: `Running`

## Timing

**Typical Latency**:
- Webhook response: < 500ms
- Image inspection: 1-5s (cached: < 100ms)
- NodeAffinity update: < 500ms
- Gate removal: < 100ms
- **Total**: 2-6s from pod creation to scheduling eligibility

## Failure Scenarios

### Image Inspection Failure

**Cause**: Registry unreachable

**Behavior**: Retry with exponential backoff (max 5 attempts)

**Fallback**: Use `fallbackArchitecture` if configured, else leave gated

### Webhook Down

**Cause**: Webhook deployment unavailable

**Behavior**: Pod creation rejected (fail-closed)

**Recovery**: Fix webhook deployment, retry pod creation

## Related Documents

- [→ Component: Pod Placement Webhook](../../design-docs/components/pod-placement-webhook.md)
- [→ Component: Pod Placement Controller](../../design-docs/components/pod-placement-controller.md)
- [→ Concept: Scheduling Gates](../concepts/scheduling-gates.md)
- [→ Concept: Image Inspection](../concepts/image-inspection.md)
