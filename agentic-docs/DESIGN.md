# Design Philosophy

## Core Beliefs

The Multiarch Tuning Operator embodies three core design principles:

###  1. Zero-Touch Multi-Architecture Support

Workloads should run on the right architecture without developer intervention. The operator automatically infers container image architectures and sets appropriate nodeAffinity constraints, enabling seamless operation in heterogeneous clusters.

**Rationale**: Developers shouldn't need to understand cluster architecture topology. The operator acts as an intelligent scheduler intermediary that matches workloads to compatible nodes based on container manifest inspection.

### 2. Safety Through Gating

The scheduling gate pattern ensures pods are never scheduled to incompatible nodes. By holding pods before scheduling, the operator has time to inspect images and compute constraints without race conditions or retry storms.

**Rationale**: Immediate scheduling of pods in multi-arch clusters risks placement on incompatible nodes, causing exec format errors. The gate provides a synchronization point for architecture analysis before scheduler involvement.

### 3. Operator-of-Operators Pattern

The operator deploys and manages subordinate operands (controller, webhook) rather than performing reconciliation directly. This separation allows independent scaling, versioning, and failure domains.

**Rationale**: Single-binary operators create coupling between lifecycle management and workload processing. The operator-of-operators pattern enables horizontal scaling of the pod placement controller (high concurrency for image inspection) while keeping the operator controller lightweight (singleton CR reconciliation).

## Architectural Decisions

### Image Inspection Over Heuristics

**Decision**: Inspect actual container manifests from registries instead of relying on labels or conventions.

**Why**: Labels can be missing or incorrect. Manifest inspection provides ground truth about supported architectures by examining the OCI/Docker manifest list.

**Trade-off**: Requires network calls to registries, adding latency. Mitigated through aggressive caching and concurrent reconciliation (NumCPU * 4 workers).

### Webhook + Controller Duality

**Decision**: Use both mutating webhook (add gate) and controller (process gate) instead of controller-only or webhook-only approaches.

**Why**: 
- Webhook alone: Cannot perform long-running image inspection (webhook timeout limits)
- Controller alone: Requires watching all pods, not just gated ones (performance impact)
- Combined: Webhook adds gate instantly, controller processes asynchronously with field selector optimization (`status.phase=Pending`)

### v1alpha1 → v1beta1 API Evolution

**Decision**: Maintain v1alpha1 for compatibility with conversion webhook to v1beta1 (storage version).

**Why**: Clusters may have existing ClusterPodPlacementConfig resources at v1alpha1. Conversion webhook ensures seamless upgrades without manual intervention.

## Extension Points

### Plugin System

The `plugins` field in ClusterPodPlacementConfig enables extensibility:

**NodeAffinityScoring Plugin**: Adds preferred (soft) nodeAffinity based on cluster node distribution, enabling workload placement hints while maintaining hard architecture constraints.

**Future plugins**: Custom architecture mapping, cost-based placement, telemetry hooks.

### Fallback Architecture

When image inspection fails (private registries, missing manifests), `fallbackArchitecture` provides graceful degradation instead of blocking pod scheduling indefinitely.

## Non-Goals

- **Not a general-purpose scheduler**: Operator only sets nodeAffinity; it does not implement scheduling algorithms. The Kubernetes scheduler handles placement.
- **Not a multi-cluster solution**: Operates within a single cluster. Multi-cluster heterogeneity is out of scope.
- **Not a security boundary**: Image inspection uses pull secrets for authentication but does not enforce registry policies. That's the admission controller's job.

## Related Documents

- [→ Component: Operator Controller](design-docs/components/operator-controller.md) - Lifecycle management
- [→ Component: Pod Placement Controller](design-docs/components/pod-placement-controller.md) - Reconciliation logic
- [→ Concept: Scheduling Gates](domain/concepts/scheduling-gates.md) - Gating mechanism
- [→ Workflow: Pod Placement Flow](domain/workflows/pod-placement.md) - End-to-end flow
