# Development Setup

## Prerequisites

**Required**:
- Go 1.20+
- Docker or Podman
- make
- gpgme development headers (`gpgme-devel` on RHEL/Fedora, `libgpgme-dev` on Debian)
- operator-sdk
- Kubernetes cluster (OpenShift 4.x recommended, vanilla Kubernetes supported)

**Optional**:
- qemu-user-static (for multi-arch image builds via `docker buildx`)

## Quick Start

```bash
# Clone repository
git clone https://github.com/openshift/multiarch-tuning-operator
cd multiarch-tuning-operator

# Build operator binary locally (requires CGO)
make build

# Run all checks and tests
make test

# Deploy to cluster
make deploy IMG=quay.io/yourorg/multiarch-tuning-operator:latest
```

##Local vs Containerized Builds

By default, builds and tests run in a containerized environment using `BUILD_IMAGE`. To run locally:

```bash
# Option 1: Environment variable
NO_DOCKER=1 make test

# Option 2: .env file (persistent)
echo "NO_DOCKER=1" > .env
make test
```

## Running Single Tests

Tests use Ginkgo framework. Target specific tests with `GINKGO_ARGS`:

```bash
# Focus on specific test pattern
GINKGO_ARGS="-v --focus='PodReconciler should add nodeAffinity'" make unit

# Run e2e tests (requires deployed operator)
KUBECONFIG=/path/to/kubeconfig NAMESPACE=openshift-multiarch-tuning-operator make e2e
```

## Binary Execution Modes

The operator binary has four mutually exclusive modes (see `cmd/main.go:bindFlags()`):

1. **--enable-operator**: Operator controller (manages ClusterPodPlacementConfig CR)
2. **--enable-ppc-controllers**: Pod placement controller (reconciles pods)
3. **--enable-ppc-webhook**: Pod placement webhook (adds scheduling gates)
4. **--enable-enoexec-event-controllers**: ENoExecEvent handler (eBPF error monitoring)

Only one mode can be active. Leader election IDs differ per mode.

## Modifying APIs

After editing `api/v1beta1/*_types.go`:

```bash
# Regenerate CRDs, RBAC, DeepCopy implementations
make manifests generate

# Verify no git diffs (important for CI)
make verify-diff
```

## Vendoring

This project uses Go vendoring (`GOFLAGS=-mod=vendor`). After changing dependencies:

```bash
go mod tidy
make vendor
```

## Image Inspector Development

Image inspection requires CGO for gpgme library. When modifying `pkg/image/inspector.go`:

- Ensure `CGO_ENABLED=1`
- Manifests are cached aggressively; clear cache in tests via `sync.Map` reset
- Multi-arch manifest lists return set of architectures; single-arch manifests return one element

## Testing Infrastructure

**Unit Tests**: Located alongside source files (`*_test.go`). Use envtest for fake Kubernetes API server.

**Test Helpers**: `pkg/testing/builder/` provides fluent builders for K8s objects:
```go
pod := builder.NewPod().
    WithName("test-pod").
    WithImage("docker.io/library/nginx:latest").
    WithSchedulingGate(common.SchedulingGateName).
    Build()
```

**E2E Tests**: Located in `pkg/e2e/`. Require deployed operator. Separate test suites for operator and pod placement.

## Debugging Tips

**Check operator mode**:
```bash
kubectl logs -n openshift-multiarch-tuning-operator deployment/multiarch-tuning-operator | grep "Starting manager"
```

**Inspect gated pods**:
```bash
kubectl get pods -A -l multiarch.openshift.io/scheduling-gate=gated
```

**View image inspection metrics**:
```bash
kubectl port-forward -n openshift-multiarch-tuning-operator deployment/multiarch-tuning-operator-pod-placement-controller 8080:8080
curl localhost:8080/metrics | grep mto_ppo_ctrl
```

## Related Documents

- [→ TESTING.md](TESTING.md) - Test strategy and coverage
- [→ Component: Pod Placement Controller](design-docs/components/pod-placement-controller.md) - Controller internals
- [→ Concept: Image Inspection](domain/concepts/image-inspection.md) - Manifest analysis
