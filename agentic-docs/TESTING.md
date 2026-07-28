# Testing Strategy

## Test Pyramid

```
        /\
       /E2\      ← E2E tests (cluster required, slower)
      /____\
     /      \
    / Unit   \   ← Unit tests (envtest, fast)
   /__________\
```

**Philosophy**: Maximize unit test coverage for business logic. E2E tests validate integration points and real cluster behavior.

## Unit Testing

**Framework**: Ginkgo + Gomega + envtest

**Coverage**: Controllers, webhooks, image inspection, pod model logic

**Key Test Suites**:
- `internal/controller/operator/*_test.go`: Operator controller reconciliation
- `internal/controller/podplacement/*_test.go`: Pod reconciliation, webhook mutation
- `pkg/image/*_test.go`: Image manifest parsing, architecture extraction

**Running Unit Tests**:
```bash
# All unit tests
make unit

# Specific test suite
GINKGO_ARGS="--focus='PodReconciler'" make unit

# With coverage report
make test  # Generates test-unit-coverage.out and test-unit-coverage.html
```

**Test Helpers**: `pkg/testing/builder/` provides fluent object builders to reduce test boilerplate:
```go
pod := builder.NewPod().WithSchedulingGate(common.SchedulingGateName).Build()
cppc := builder.NewClusterPodPlacementConfig().WithLogVerbosity(common.Debug).Build()
```

## E2E Testing

**Requirements**: Deployed operator in cluster (via `make deploy`)

**Coverage**: Full pod placement workflow, ClusterPodPlacementConfig lifecycle, multi-arch scenarios

**Key Test Suites**:
- `pkg/e2e/operator/`: ClusterPodPlacementConfig CR creation, status updates
- `pkg/e2e/podplacement/`: Pod gating, architecture inference, nodeAffinity injection

**Running E2E Tests**:
```bash
# Ensure operator is deployed
make deploy IMG=quay.io/yourorg/multiarch-tuning-operator:latest

# Run E2E tests
KUBECONFIG=/path/to/kubeconfig NAMESPACE=openshift-multiarch-tuning-operator make e2e
```

**E2E Test Scenarios**:
1. **Basic pod placement**: Pod with multi-arch image → nodeAffinity set → scheduling gate removed
2. **Fallback architecture**: Image inspection fails → fallback architecture used (if configured)
3. **Namespace exclusion**: Pod in excluded namespace → no gate added
4. **Operator upgrade**: v1alpha1 → v1beta1 CR conversion
5. **ENoExecEvent handling**: Exec format error detected → event recorded

## Image Inspection Testing

**Challenge**: Image inspection requires network calls to container registries.

**Approach**:
- **Unit tests**: Mock registry responses using `pkg/testing/registry/` test server
- **E2E tests**: Use public multi-arch images (e.g., `docker.io/library/nginx`)

**Test Coverage**:
- Multi-arch manifest lists (return architecture set)
- Single-arch manifests (return single architecture)
- Operator bundle images (return empty set - architecture-agnostic)
- Authentication (pull secrets, global pull secret)
- Caching behavior (repeated inspections return cached results)

## Envtest Limitations

**What envtest provides**: Fake Kubernetes API server (CRDs, RBAC, basic operations)

**What envtest does NOT provide**:
- Scheduler (cannot test actual pod placement)
- Webhooks (tested separately via webhook test harness)
- Node architecture diversity (simulated via labels in tests)

**Workarounds**:
- Mock scheduler behavior by checking nodeAffinity in pod spec
- Validate webhook logic directly without admission chain
- Create fake nodes with architecture labels for testing

## Continuous Integration

**CI Pipeline** (`.tekton/` for Konflux):
1. `make lint gosec vet goimports fmt` - Code quality checks
2. `make unit` - Unit test suite (containerized)
3. `make e2e` - E2E tests (requires CI cluster)
4. `make verify-diff` - Ensure no uncommitted generated files

**Coverage Threshold**: Target 70%+ for controller logic, 60%+ overall

## Test Data Builders

Fluent builders in `pkg/testing/builder/`:

```go
// Pod with scheduling gate and multi-container spec
pod := builder.NewPod().
    WithNamespace("test-ns").
    WithName("test-pod").
    WithImage("docker.io/library/nginx:latest").
    WithSchedulingGate(common.SchedulingGateName).
    WithSecondaryImages("docker.io/library/redis:latest").
    Build()

// ClusterPodPlacementConfig with namespace selector
cppc := builder.NewClusterPodPlacementConfig().
    WithNamespaceSelector(&metav1.LabelSelector{
        MatchExpressions: []metav1.LabelSelectorRequirement{{
            Key: "multiarch.openshift.io/exclude-pod-placement",
            Operator: metav1.LabelSelectorOpDoesNotExist,
        }},
    }).
    Build()
```

## Related Documents

- [→ DEVELOPMENT.md](DEVELOPMENT.md) - Setup and build instructions
- [→ Component: Pod Placement Controller](design-docs/components/pod-placement-controller.md) - Controller testing
- [→ Concept: Scheduling Gates](domain/concepts/scheduling-gates.md) - Gate mechanics
