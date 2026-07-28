# Concept: ClusterPodPlacementConfig API

## Overview

ClusterPodPlacementConfig is the singleton custom resource that controls the pod placement operand. Only one instance allowed (name: `cluster`).

## API Versions

- **v1alpha1**: Original API (deprecated, conversion webhook to v1beta1)
- **v1beta1**: Current stable API (storage version)

## Spec Fields

### logVerbosity

**Type**: `string`  
**Values**: `Normal`, `Debug`, `Trace`, `TraceAll`  
**Default**: `Normal`

**Purpose**: Control log verbosity for pod placement components

**Examples**:
- `Normal`: Info-level logs, minimal output
- `Debug`: Debug-level logs, includes reconciliation decisions
- `Trace`: Trace-level logs, includes image inspection details
- `TraceAll`: Trace all operations, very verbose

### namespaceSelector

**Type**: `metav1.LabelSelector`  
**Default**: `nil` (all namespaces)

**Purpose**: Select which namespaces should have pod placement enabled

**Example** (exclude labeled namespaces):
```yaml
namespaceSelector:
  matchExpressions:
    - key: multiarch.openshift.io/exclude-pod-placement
      operator: DoesNotExist
```

**Hard-Coded Exclusions** (always excluded, cannot be overridden):
- `openshift-*`
- `kube-*`
- `hypershift-*`

### plugins

**Type**: `plugins.Plugins`  
**Optional**: true

**Purpose**: Configure optional plugins (e.g., NodeAffinityScoring)

**Example**:
```yaml
plugins:
  nodeAffinityScoring:
    enabled: true
```

### fallbackArchitecture

**Type**: `string`  
**Values**: `arm64`, `amd64`, `ppc64le`, `s390x`, `""` (empty)  
**Default**: `""` (no fallback)

**Purpose**: Architecture to use if image inspection fails

**Behavior**:
- If set: Use fallback architecture, ungate pod
- If not set: Leave pod gated indefinitely on inspection failure

## Status Fields

### conditions

**Type**: `[]metav1.Condition`

**Condition Types**:
- `Available`: All operands running and ready
- `Progressing`: Operands being deployed/updated
- `Degraded`: Errors in operand deployment
- `Deprovisioning`: CR being deleted, operands being removed

**Example**:
```yaml
status:
  conditions:
    - type: Available
      status: "True"
      reason: AllOperandsReady
      message: "Pod placement controller and webhook are ready"
    - type: Progressing
      status: "False"
      reason: StableState
      message: "No operand updates in progress"
```

## Singleton Constraint

**Webhook Validation**: Only name `cluster` allowed

**Rationale**: Global cluster configuration, multiple instances would create conflicts

**Creation**:
```bash
kubectl create -f - <<EOF
apiVersion: multiarch.openshift.io/v1beta1
kind: ClusterPodPlacementConfig
metadata:
  name: cluster  # MUST be "cluster"
spec:
  logVerbosity: Normal
EOF
```

**Rejection Example**:
```bash
kubectl create -f - <<EOF
apiVersion: multiarch.openshift.io/v1beta1
kind: ClusterPodPlacementConfig
metadata:
  name: my-config  # Will be rejected
spec:
  logVerbosity: Normal
EOF
# Error: ClusterPodPlacementConfig name must be "cluster"
```

## Related Documents

- [→ Component: Operator Controller](../../design-docs/components/operator-controller.md) - Reconciles CPPC
- [→ Concept: CPPC Informer](cppc-informer.md) - Singleton syncer
- [→ API Reference](https://github.com/openshift/multiarch-tuning-operator/blob/main/api/v1beta1/clusterpodplacementconfig_types.go)
