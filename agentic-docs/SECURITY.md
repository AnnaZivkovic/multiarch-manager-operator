# Security Model

## Threat Model

### Assets

1. **ClusterPodPlacementConfig CR**: Singleton configuration resource (name: `cluster`)
2. **Pod Specs**: Workload specifications subject to mutation (nodeAffinity injection)
3. **Pull Secrets**: Container registry credentials used for image inspection
4. **Webhook TLS Certificates**: Authentication for mutating webhook

### Threats

**T1: Malicious Pod Creation**: Attacker creates pods designed to bypass scheduling gates

**Mitigation**: Webhook runs in fail-closed mode. If webhook is down, pod creation fails rather than allowing unvalidated pods.

**T2: Pull Secret Exfiltration**: Operator inspects images using pull secrets; compromised operator could leak credentials

**Mitigation**: 
- Operator runs with minimal RBAC (read-only access to secrets, scoped to namespaces)
- Pull secrets used only for transient image inspection, not persisted
- Audit logging enabled for secret access

**T3: Privilege Escalation via ClusterPodPlacementConfig**: Attacker modifies CPPC to bypass namespace exclusions

**Mitigation**:
- CPPC is cluster-scoped resource requiring cluster-admin privileges
- Webhook validates CPPC name (must be "cluster")
- System namespaces (`openshift-*`, `kube-*`, `hypershift-*`) cannot be included via namespaceSelector (hard-coded exclusion)

**T4: Webhook Certificate Compromise**: Attacker replaces webhook certificate with malicious cert

**Mitigation**:
- Cert-manager handles certificate lifecycle (auto-rotation)
- Webhook TLS certificate stored in Kubernetes secret with restricted RBAC
- Certificate validation by API server before webhook invocation

## RBAC Permissions

### Operator Controller

```yaml
# ClusterRole: multiarch-tuning-operator
rules:
  - apiGroups: ["multiarch.openshift.io"]
    resources: ["clusterpodplacementconfigs"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["serviceaccounts", "services", "configmaps"]
    verbs: ["create", "get", "list", "watch", "update", "patch", "delete"]
```

**Principle**: Least privilege for operand deployment. No direct pod mutation.

### Pod Placement Controller

```yaml
# ClusterRole: multiarch-tuning-operator-pod-placement-controller
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]  # Read-only for pull secrets
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch"]
```

**Principle**: Read-only secret access. Pod update limited to nodeAffinity and scheduling gates.

### Pod Placement Webhook

```yaml
# ClusterRole: multiarch-tuning-operator-pod-placement-webhook
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch"]
```

**Principle**: Webhook modifies pods via admission chain, not direct API calls. Minimal RBAC.

## Image Inspection Security

### Pull Secret Handling

**Flow**:
1. Controller reads pod's `imagePullSecrets` references
2. Fetches secret content from pod's namespace
3. Combines with global pull secret (if configured)
4. Creates temporary auth file for containers/image library
5. **Deletes auth file after inspection** (no persistence)

**Constraints**:
- Operator cannot access secrets outside namespaces with gated pods
- Auth files use unique temporary paths (`/tmp/auth-<uuid>.json`)
- No secret content logged (even at Trace verbosity)

### Registry Communication

**TLS**: All registry connections use TLS (verified certificates)

**Authentication**: 
- Docker config JSON format (`.dockerconfigjson`)
- Global pull secret at `openshift-config/pull-secret` (OpenShift)
- Per-pod pull secrets (any namespace)

**Timeout**: 10s default for image inspection (prevents indefinite hangs on slow/malicious registries)

## Admission Control

### Mutating Webhook Configuration

```yaml
failurePolicy: Fail  # Fail-closed: reject pods if webhook is down
sideEffects: None
admissionReviewVersions: ["v1"]
timeoutSeconds: 10
```

**Rationale**: Fail-closed prevents pods from bypassing scheduling gates if webhook is unavailable.

**Risk**: Pod creation blocked during webhook outages. Mitigated by horizontal webhook scaling and liveness probes.

### Webhook TLS

**Certificate Authority**: Kubernetes CA (via cert-manager)
**Certificate Rotation**: Automatic (30 days before expiration)
**Certificate Storage**: Kubernetes secret (`multiarch-tuning-operator-pod-placement-webhook-cert`)

## Namespace Isolation

### Hard-Coded Exclusions

System namespaces are **always excluded** from pod placement (cannot be overridden):
- `openshift-*`
- `kube-*`
- `hypershift-*`

**Rationale**: Critical cluster components must not be subject to operator mutation. Exec format errors in these namespaces could cause cluster instability.

### User-Defined Exclusions

Users can exclude additional namespaces via label:
```yaml
metadata:
  labels:
    multiarch.openshift.io/exclude-pod-placement: ""
```

## Security Scanning

**SAST**: `make gosec` - Static analysis security testing

**Dependency Scanning**: Dependabot checks for vulnerable dependencies

**Container Scanning**: Konflux CI scans operator images for CVEs

## Related Documents

- [→ Component: Pod Placement Webhook](design-docs/components/pod-placement-webhook.md) - Webhook admission logic
- [→ Component: Pod Placement Controller](design-docs/components/pod-placement-controller.md) - Pull secret handling
- [→ Concept: Image Inspection](domain/concepts/image-inspection.md) - Authentication flow
