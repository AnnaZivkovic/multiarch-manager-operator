# Concept: Image Inspection

## Overview

Image inspection determines which CPU architectures a container image supports by examining its manifest from the registry. This information drives architecture-aware pod placement.

## Inspection Process

### 1. Image Reference Parsing

Extract components from image reference:

```
docker.io/library/nginx:1.25
└─┬──┘  └───┬──┘ └─┬┘ └─┬┘
  │         │      │    └── Tag
  │         │      └────── Repository
  │         └───────────── Namespace
  └─────────────────────── Registry
```

### 2. Authentication Resolution

Build auth file from secrets (priority order):
1. Pod's `imagePullSecrets`
2. ServiceAccount's `imagePullSecrets`
3. Global pull secret

### 3. Manifest Retrieval

Fetch manifest from registry using containers/image library:

```go
imageRef, _ := docker.ParseReference("//docker.io/library/nginx:latest")
manifest, _ := imageRef.Manifest(ctx, &types.SystemContext{AuthFilePath: "/tmp/auth.json"})
```

### 4. Architecture Extraction

**Multi-Arch Manifest List** (OCI Index or Docker Manifest List):
```json
{
  "manifests": [
    {"platform": {"architecture": "amd64"}},
    {"platform": {"architecture": "arm64"}},
    {"platform": {"architecture": "ppc64le"}}
  ]
}
```
→ Return `{amd64, arm64, ppc64le}`

**Single-Arch Manifest**:
```json
{
  "config": {"digest": "sha256:abc..."}
}
```
→ Fetch config blob → Extract `architecture` field → Return `{amd64}`

## Supported Architectures

The operator recognizes these architectures:
- `amd64` (x86_64)
- `arm64` (aarch64)
- `ppc64le` (POWER9+)
- `s390x` (IBM Z)

## Caching Strategy

**Cache Key**: Full image reference including digest/tag

**Cache Duration**: Lifetime of pod placement controller process

**Cache Hit Ratio**: Expected >90% in steady-state (repeated deployments of same images)

**Metrics**: `mto_ppo_ctrl_time_to_inspect_image_seconds` includes cache lookups

## Failure Scenarios

### Registry Unreachable

**Cause**: Network partition, DNS failure, registry down

**Behavior**: Retry with exponential backoff (max 5 attempts)

**Fallback**: Use `fallbackArchitecture` from ClusterPodPlacementConfig (if configured)

### Authentication Failure

**Cause**: Invalid pull secret, expired token, missing secret

**Behavior**: Return error to PodReconciler

**Recovery**: Fix pull secret, pod will be re-reconciled

### Image Not Found

**Cause**: Typo in image reference, image deleted, wrong registry

**Behavior**: Permanent error, no retries

**Recovery**: Fix pod spec image reference, recreate pod

## Special Cases

### Operator Bundle Images

Operator bundles (OLM) are architecture-agnostic. Detected via annotations:

```json
"operators.operatorframework.io.bundle.mediatype.v1": "registry+v1"
```

**Return**: Empty set `{}` → No architecture constraints added

### Digests vs Tags

**Digest reference** (`nginx@sha256:abc...`): Immutable, points to specific manifest

**Tag reference** (`nginx:latest`): Mutable, can point to different manifests over time

**Cache invalidation**: Tags not invalidated (assumes tag stability during controller lifetime)

## Related Documents

- [→ Component: Image Inspector](../../design-docs/components/image-inspector.md) - Implementation
- [→ Component: Pod Placement Controller](../../design-docs/components/pod-placement-controller.md) - Consumer
- [→ Concept: Container Registries](container-registries.md) - Registry interaction
