# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The Multiarch Tuning Operator enhances operational experience within multi-architecture clusters by providing architecture-aware pod placement. It is a Kubernetes operator built with Kubebuilder that manages the pod placement operand consisting of a controller and webhook.

## Development Commands

### Build Commands
```shell
# Local build (CGO required - needs gpgme-devel/libgpgme-dev)
make build

# Single-architecture image build
make docker-build IMG=<registry>/multiarch-tuning-operator:tag

# Multi-architecture image build (requires qemu-user-static)
make docker-buildx IMG=<registry>/multiarch-tuning-operator:tag

# Prevent buildx instance deletion (create empty file)
touch .persistent-buildx
```

### Code Quality and Testing
```shell
# Run all checks and tests (includes manifests, generate, fmt, vet, goimports, gosec, lint, and unit tests)
make test

# Individual checks (all run in containerized environment by default)
make lint          # golangci-lint
make gosec         # SAST security scanning
make vet           # go vet
make goimports     # import formatting
make fmt           # go fmt
make unit          # unit tests only

# Run tests locally (without Docker)
NO_DOCKER=1 make test

# Run e2e tests (requires deployed operator)
KUBECONFIG=/path/to/cluster/kubeconfig NAMESPACE=openshift-multiarch-tuning-operator make e2e

# Generate manifests after API changes
make manifests

# Update vendored dependencies
make vendor

# Verify no uncommitted changes in working tree
make verify-diff
```

### Deployment Commands
```shell
# Install CRDs
make install

# Deploy operator to cluster
make deploy IMG=<registry>/multiarch-tuning-operator:tag

# Create ClusterPodPlacementConfig to enable pod placement operand
kubectl create -f - <<EOF
apiVersion: multiarch.openshift.io/v1beta1
kind: ClusterPodPlacementConfig
metadata:
  name: cluster
spec:
  logVerbosityLevel: Normal
  namespaceSelector:
    matchExpressions:
      - key: multiarch.openshift.io/exclude-pod-placement
        operator: DoesNotExist
EOF

# Undeploy
make undeploy

# Uninstall CRDs
make uninstall
```

### Bundle and Catalog Commands
```shell
# Generate bundle manifests
make bundle VERSION=<version>

# Verify bundle generation is deterministic
make bundle-verify

# Build and push bundle image
make bundle-build BUNDLE_IMG=<registry>/multiarch-tuning-operator-bundle:<version>
make bundle-push BUNDLE_IMG=<registry>/multiarch-tuning-operator-bundle:<version>

# Build and push catalog image
make catalog-build CATALOG_IMG=<registry>/multiarch-tuning-operator-catalog:<version>
make catalog-push CATALOG_IMG=<registry>/multiarch-tuning-operator-catalog:<version>
```

## Architecture

### Execution Modes

The operator binary runs in three mutually exclusive modes (controlled by flags in main.go):

1. **Operator Mode** (`--enable-operator`): Manages the ClusterPodPlacementConfig CR lifecycle. Deploys and manages the pod placement operand components (controller and webhook deployments).

2. **Pod Placement Controller Mode** (`--enable-ppc-controllers`): Reconciles pods with the scheduling gate, inspects container images to determine supported architectures, sets nodeAffinity requirements, and removes the scheduling gate.

3. **Pod Placement Webhook Mode** (`--enable-ppc-webhook`): Mutating webhook that adds the scheduling gate to new pods (except in excluded namespaces).

### Key Components

**Operator Controller** (`controllers/operator/clusterpodplacementconfig_controller.go`):
- Reconciles ClusterPodPlacementConfig singleton resource (name must be "cluster")
- Manages deployment lifecycle of pod placement controller and webhook
- Updates status conditions (Available, Progressing, Degraded, Deprovisioning)

**Pod Placement Controller** (`controllers/podplacement/pod_reconciler.go`):
- Watches pods in Pending status with the `multiarch.openshift.io/scheduling-gate` scheduling gate
- High concurrency: `MaxConcurrentReconciles = NumCPU * 4` (I/O bound image inspection)
- Reconciliation flow:
  1. Check if pod has scheduling gate
  2. Verify pod should be processed (namespace selector, existing nodeAffinity)
  3. Retrieve image pull secrets
  4. Inspect container images to determine supported architectures
  5. Set required and preferred nodeAffinity for `kubernetes.io/arch`
  6. Remove scheduling gate to allow scheduling
- Max retries mechanism for image inspection failures
- Cache optimization in pod field selector: only watches `status.phase=Pending`

**Pod Model** (`controllers/podplacement/pod_model.go`):
- Core logic for pod processing
- Image architecture inspection (supports registry authentication)
- NodeAffinity computation (required and preferred scheduling)
- Scheduling gate management
- Event publishing for audit trail

**Mutating Webhook** (`controllers/podplacement/scheduling_gate_mutating_webhook.go`):
- Adds `multiarch.openshift.io/scheduling-gate` to new pods
- Respects namespace selector from ClusterPodPlacementConfig
- Always excludes: `openshift-*`, `kube-*`, `hypershift-*` namespaces
- Uses worker pool for event publishing (ants library, 16 workers)

**Image Inspector** (`pkg/image/inspector.go`):
- Retrieves container image manifests from registries
- Determines supported CPU architectures
- Handles authentication via pull secrets and global pull secret
- Implements caching to reduce registry queries

**CPPC Informer** (`pkg/informers/clusterpodplacementconfig/`):
- Syncer that maintains in-memory singleton of ClusterPodPlacementConfig
- Enabled in operand modes (`--enable-cppc-informer`)
- Allows runtime log level changes and configuration access

### API Versions

The operator supports multiple API versions with conversion webhooks:
- `v1alpha1`: Initial version (conversion webhook to v1beta1)
- `v1beta1`: Storage version with Plugins support

**ClusterPodPlacementConfig** (`apis/multiarch/v1beta1/clusterpodplacementconfig_types.go`):
- Singleton resource (only name "cluster" allowed)
- Spec fields:
  - `logVerbosity`: Normal, Debug, Trace, TraceAll
  - `namespaceSelector`: Label selector for processed namespaces
  - `plugins`: Optional plugins configuration (e.g., NodeAffinityScoring for preferred scheduling)
- Status conditions: Available, Progressing, Degraded, Deprovisioning, PodPlacementControllerNotRolledOut, PodPlacementWebhookNotRolledOut, MutatingWebhookConfigurationNotAvailable

### Plugins System

**NodeAffinityScoring Plugin** (`apis/multiarch/common/plugins/nodeaffinityscoring_plugin.go`):
- Adds preferred (soft) nodeAffinity to influence scheduler scoring
- Weights architectures based on cluster node distribution
- Enables workload placement on preferred architectures while maintaining required constraints

## Code Organization

```
apis/multiarch/
├── common/              # Shared constants and types
│   └── plugins/         # Plugin system (NodeAffinityScoring)
├── v1alpha1/            # Alpha API version with conversion
└── v1beta1/             # Beta API version (storage version)

controllers/
├── operator/            # Operator mode: ClusterPodPlacementConfig lifecycle
└── podplacement/        # Operand modes: pod reconciler and webhook
    └── metrics/         # Prometheus metrics

pkg/
├── e2e/                 # E2E test utilities
├── image/               # Container image inspection and authentication
├── informers/           # CPPC singleton informer
├── testing/             # Test helpers
└── utils/               # Shared utilities

config/                  # Kustomize configurations
hack/                    # Build and test scripts
```

## Testing

### Unit Tests
- Framework: Ginkgo v2 with Gomega matchers
- Run with: `make unit` or `NO_DOCKER=1 make unit`
- Test files: `*_test.go` throughout the codebase
- Key test suites:
  - `controllers/operator/suite_test.go`: Operator controller tests
  - `controllers/podplacement/suite_test.go`: Pod placement tests
  - `controllers/podplacement/pod_model_test.go`: Core pod processing logic tests

### E2E Tests
- Label: `e2e` (filtered via Ginkgo label-filter)
- Run with: `KUBECONFIG=... NAMESPACE=openshift-multiarch-tuning-operator make e2e`
- Test manifests in: `test/manifests/`

### Test Configuration
- All tests run in containers by default using `BUILD_IMAGE`
- Set `NO_DOCKER=1` in `.env` file or environment to run locally
- Test artifacts output to `ARTIFACT_DIR` (default: `./_output`)
- Coverage reports generated in `test-unit-coverage.out`

## Environment Configuration

Create a `.env` file in the repository root for local settings:
```
NO_DOCKER=1                    # Run tests and builds locally
FORCE_DOCKER=1                 # Force Docker instead of Podman
BUILD_IMAGE=<custom-image>     # Override builder image
RUNTIME_IMAGE=<custom-image>   # Override runtime base image
```

## Important Constraints

- ClusterPodPlacementConfig must be named "cluster" (singleton enforced by webhook)
- Namespaces `openshift-*`, `kube-*`, and `hypershift-*` are always excluded from pod placement
- CGO is required for building (uses gpgme for registry authentication)
- Only one execution mode flag can be set at a time in main.go
- The operator uses vendored dependencies (`GOFLAGS=-mod=vendor`)

## Metrics

All components expose Prometheus metrics at `:8080/metrics`:

**Pod Placement Controller**:
- `mto_ppo_ctrl_time_to_process_pod_seconds`: Time to process any pod
- `mto_ppo_ctrl_time_to_process_gated_pod_seconds`: Time to process gated pods (includes inspection)
- `mto_ppo_ctrl_time_to_inspect_image_seconds`: Image inspection time
- `mto_ppo_ctrl_processed_pods_total`: Total gated pods processed
- `mto_ppo_ctrl_failed_image_inspection_total`: Failed image inspections

**Mutating Webhook**:
- `mto_ppo_wh_pods_processed_total`: Total pods processed
- `mto_ppo_wh_pods_gated_total`: Total pods gated
- `mto_ppo_wh_response_time_seconds`: Webhook response time

**Shared**:
- `mto_ppo_pods_gated`: Current number of gated pods

See `docs/metrics.md` for example queries and monitoring setup.

## Related Documentation

- [OpenShift Enhancement Proposal](https://github.com/openshift/enhancements/blob/6cebc13f0672c601ebfae669ea4fc8ca632721b5/enhancements/multi-arch/multiarch-manager-operator.md)
- [KEP-3521: Pod Scheduling Readiness](https://github.com/kubernetes/enhancements/blob/afad6f270c7ac2ae853f4d1b72c379a6c3c7c042/keps/sig-scheduling/3521-pod-scheduling-readiness/README.md)
- [KEP-3838: Pod Mutable Scheduling Directives](https://github.com/kubernetes/enhancements/blob/afad6f270c7ac2ae853f4d1b72c379a6c3c7c042/keps/sig-scheduling/3838-pod-mutable-scheduling-directives/README.md)
- [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
