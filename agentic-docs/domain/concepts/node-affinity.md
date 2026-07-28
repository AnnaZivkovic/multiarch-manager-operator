# Concept: Node Affinity

## Overview

Node affinity is a Kubernetes scheduling constraint that limits which nodes a pod can be placed on. The Multiarch Tuning Operator sets architecture-based nodeAffinity to match workloads to compatible nodes.

## Affinity Types

### Required Affinity (Hard Constraint)

**Field**: `requiredDuringSchedulingIgnoredDuringExecution`

**Behavior**: Scheduler MUST place pod on node matching constraint, or pod remains unscheduled

**Operator Usage**: Set required affinity for `kubernetes.io/arch` based on image architectures

**Example**:
```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: kubernetes.io/arch
                operator: In
                values: ["amd64", "arm64"]  # Pod can run on amd64 OR arm64 nodes
```

### Preferred Affinity (Soft Constraint)

**Field**: `preferredDuringSchedulingIgnoredDuringExecution`

**Behavior**: Scheduler PREFERS nodes matching constraint but will schedule elsewhere if needed

**Operator Usage**: Set preferred affinity when NodeAffinityScoring plugin enabled (weights based on cluster node distribution)

**Example**:
```yaml
spec:
  affinity:
    nodeAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100  # Higher weight = stronger preference
          preference:
            matchExpressions:
              - key: kubernetes.io/arch
                operator: In
                values: ["amd64"]  # Prefer amd64 nodes
```

## Operator Affinity Injection

### Single-Arch Image

**Image**: `nginx:latest` (amd64 only)

**Injected nodeAffinity**:
```yaml
requiredDuringSchedulingIgnoredDuringExecution:
  nodeSelectorTerms:
    - matchExpressions:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]
```

**Effect**: Pod can ONLY be scheduled on amd64 nodes

### Multi-Arch Image

**Image**: `nginx:latest` (amd64, arm64, ppc64le, s390x)

**Injected nodeAffinity**:
```yaml
requiredDuringSchedulingIgnoredDuringExecution:
  nodeSelectorTerms:
    - matchExpressions:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64", "arm64", "ppc64le", "s390x"]
```

**Effect**: Pod can be scheduled on any architecture supported by image

### Preferred Architecture (Plugin Enabled)

**Cluster**: 10 amd64 nodes, 2 arm64 nodes

**Injected nodeAffinity** (in addition to required):
```yaml
preferredDuringSchedulingIgnoredDuringExecution:
  - weight: 100
    preference:
      matchExpressions:
        - key: kubernetes.io/arch
          operator: In
          values: ["amd64"]  # Prefer majority architecture
```

**Effect**: Scheduler scores amd64 nodes higher, but will use arm64 if needed

## Affinity Merging

### Existing nodeAffinity in Pod Spec

**Pod Spec** (user-provided):
```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
          - matchExpressions:
              - key: node.kubernetes.io/instance-type
                operator: In
                values: ["m5.large"]
```

**Operator Behavior**: **DO NOT modify** pod if it already has nodeAffinity

**Rationale**: Respect user intent. Merging affinity rules risks conflicts (e.g., user wants arm64-only, operator adds amd64).

## Architecture Label

**Node Label**: `kubernetes.io/arch`

**Values**: `amd64`, `arm64`, `ppc64le`, `s390x`, `riscv64`

**Set By**: Kubelet (automatically populated based on node hardware)

**Example**:
```yaml
apiVersion: v1
kind: Node
metadata:
  labels:
    kubernetes.io/arch: "arm64"
```

## Related Documents

- [→ Component: Pod Placement Controller](../../design-docs/components/pod-placement-controller.md) - Affinity injection logic
- [→ Concept: NodeAffinityScoring Plugin](nodeaffinity-scoring-plugin.md) - Preferred affinity plugin
- [→ Workflow: Pod Placement Flow](../workflows/pod-placement.md) - End-to-end flow
