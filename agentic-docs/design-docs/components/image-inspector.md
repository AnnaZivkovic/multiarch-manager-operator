# Component: Image Inspector

**Location**: `pkg/image/inspector.go`  
**Purpose**: Inspect container images to determine supported CPU architectures

## Responsibilities

1. **Fetch Image Manifests**: Retrieve manifests from container registries (OCI, Docker v2.2)
2. **Parse Manifest Lists**: Extract architecture support from multi-arch manifests
3. **Handle Authentication**: Use pull secrets and global pull secret for registry access
4. **Cache Results**: Avoid repeated registry calls for same image
5. **Return Architecture Set**: Provide set of supported architectures (e.g., `{amd64, arm64}`)

## Key Files

- `inspector.go`: Main inspector implementation
- `auth.go`: Authentication file creation (Docker config.json format)
- `cache.go`: In-memory caching of inspection results

## Inspection Flow

```go
func (i *registryInspector) GetCompatibleArchitecturesSet(
    ctx context.Context, 
    imageReference string, 
    _ bool, 
    secrets [][]byte,
) (sets.Set[string], error) {
    // 1. Create auth file from secrets
    // 2. Parse image reference (registry, repository, tag/digest)
    // 3. Fetch image manifest from registry
    // 4. If manifest list (multi-arch):
    //    - Extract architectures from all manifests
    //    - Return set {arch1, arch2, ...}
    // 5. If single manifest:
    //    - Extract architecture from config
    //    - Return set {arch}
    // 6. If operator bundle image (detected via annotations):
    //    - Return empty set (bundles are architecture-agnostic)
    // 7. Cache result
    // 8. Clean up auth file
}
```

## Manifest Types

### Multi-Arch Manifest List (OCI Index)

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:abc123...",
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:def456...",
      "platform": {
        "architecture": "arm64",
        "os": "linux"
      }
    }
  ]
}
```

**Extraction**: Return `{amd64, arm64}`

### Single-Arch Manifest

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:ghi789..."
  }
}
```

**Extraction**: Fetch config blob, parse `architecture` field → Return `{amd64}`

### Operator Bundle Image

Detected via annotations:
```json
{
  "annotations": {
    "operators.operatorframework.io.bundle.mediatype.v1": "registry+v1",
    "operators.operatorframework.io.bundle.manifests.v1": "manifests/"
  }
}
```

**Extraction**: Return `{}` (empty set - bundles are not tied to specific architectures)

## Authentication

**Pull Secret Resolution** (priority order):
1. Pod's `imagePullSecrets` (per-pod secrets)
2. ServiceAccount's `imagePullSecrets` (inherited by pod)
3. Global pull secret (`openshift-config/pull-secret` on OpenShift)

**Auth File Format** (Docker config.json):
```json
{
  "auths": {
    "registry.example.com": {
      "auth": "base64(username:password)"
    },
    "quay.io": {
      "auth": "base64(token)"
    }
  }
}
```

**Lifecycle**:
1. Create temporary file: `/tmp/auth-<uuid>.json`
2. Write merged secrets to file
3. Pass file path to containers/image library
4. **Delete file immediately after inspection** (no persistence)

## Caching

**Cache Structure**: `sync.Map` (thread-safe in-memory map)

**Cache Key**: Image reference (e.g., `docker.io/library/nginx:latest`)

**Cache Value**: Architecture set (e.g., `{amd64, arm64}`)

**Cache Invalidation**: No TTL - cache lives for pod placement controller lifetime

**Rationale**: Image architectures rarely change for a given tag. Aggressive caching reduces registry load.

## Error Handling

**Transient Errors** (retryable):
- Registry unreachable (network timeout)
- Authentication failure (invalid pull secret)
- Rate limiting (HTTP 429)

**Permanent Errors** (not retryable):
- Image not found (HTTP 404)
- Manifest parse error (invalid JSON)
- Unsupported manifest type

**Behavior on Error**:
- Return error to caller (PodReconciler)
- Caller retries with exponential backoff (max 5 attempts)
- If max retries exceeded:
  - Use `fallbackArchitecture` if configured
  - Else: leave pod gated indefinitely

## Libraries Used

- `github.com/containers/image/v5`: Core image inspection library
- `github.com/opencontainers/go-digest`: Digest parsing
- `github.com/opencontainers/image-spec`: OCI spec types

**CGO Dependency**: containers/image requires gpgme library (signature verification)

## Metrics

Metrics are published by the PodReconciler:

| Metric | Type | Description |
|--------|------|-------------|
| `mto_ppo_ctrl_time_to_inspect_image_seconds` | Histogram | Single image inspection time |
| `mto_ppo_ctrl_failed_image_inspection_total` | Counter | Failed inspections |

## Related Components

- [→ Pod Placement Controller](pod-placement-controller.md) - Consumer of inspector
- [→ Global Pull Secret Syncer](global-pull-secret-syncer.md) - Pull secret provider

## Related Concepts

- [→ Image Inspection](../../domain/concepts/image-inspection.md) - Detailed inspection flow
- [→ Container Registries](../../domain/concepts/container-registries.md) - Registry interaction
