---
title: Migrate Multiarch Tuning Operator from RHEL 9 to RHEL 10
authors:
  - "@AnnaZivkovic"
reviewers:
  - "@aleskandro"
  - "@jeffdyoung"
  - "@Prashanth684"
  - "@lwan-wanglin"
approvers:
  - "@aleskandro"
  - "@Prashanth684"
creation-date: 2026-09-30
last-updated: 2026-09-30
tracking-link: []
see-also:
  - "/docs/enhancements/MTO-0001.md"
---

# Migrate Multiarch Tuning Operator from RHEL 9 to RHEL 10

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Multiarch Tuning Operator (MTO) currently builds and ships on RHEL 9 base images
(UBI 9). RHEL 10 is GA as of June 2025 (currently at version 10.2), and OpenShift is
transitioning its operator ecosystem to RHEL 10 base images for OCP 4.22+. This
enhancement covers the full migration of MTO's build infrastructure, container images,
CI/CD pipelines, and release tooling from RHEL 9 to RHEL 10.

The operator's Go code is RHEL-version-agnostic — no application-level code changes are
required. The migration is entirely an infrastructure and packaging concern spanning
Dockerfiles, Konflux pipelines, CI operator configuration, CPE labels, upgrade automation
scripts, and FIPS build flags.

## Motivation

- RHEL 10 is the current supported enterprise Linux platform from Red Hat. Shipping on
  RHEL 9 base images means the operator runtime does not benefit from RHEL 10's updated
  userspace libraries, kernel (6.12), glibc (2.39), and security fixes.
- OpenShift 4.22+ is driving the RHEL 10 transition across the operator ecosystem.
  Operators that remain on RHEL 9 will fall behind the platform's lifecycle.
- The Brew-hosted `openshift-golang-builder` images (`brew.registry.redhat.io/rh-osbs/`)
  are being deprecated by **October 15, 2026** (ART-14300). The Konflux Dockerfile must
  migrate to `registry.redhat.io` regardless of RHEL version — combining both migrations
  reduces churn.
- Red Hat Product Security requires CPE labels to match the RHEL version the binary runs
  on for accurate CVE tracking.

### User Stories

- As a platform engineer responsible for MTO releases, I want the operator's container
  images to be built on RHEL 10 base images so that they align with the OCP 4.22+
  platform lifecycle and receive RHEL 10 security updates.
- As an MTO developer, I want the CI/CD infrastructure and upgrade automation scripts
  updated so that Go version bumps and OCP version bumps continue to work correctly
  against RHEL 10 image references.

### Goals

- Migrate all container base images from UBI 9 to UBI 10.
- Update CPE labels from `el9` to `el10`.
- Update CI operator config, Konflux Dockerfiles, and Tekton pipelines for RHEL 10.
- Update upgrade automation scripts to use RHEL 10 image patterns.
- Ensure FIPS compliance under RHEL 10's crypto stack.
- Validate that the CGO dependency chain (`gpgme-devel`) works on RHEL 10.
- Validate multi-architecture builds (amd64, arm64, ppc64le, s390x) on RHEL 10.

### Non-Goals

- Changing the operator's Go application code. The Go code is RHEL-version-agnostic.
- Maintaining a dual RHEL 9 / RHEL 10 build. This is a one-way migration on the main
  branch.
- Upgrading the Go version. The current Go 1.26.7 is available in UBI 10's `go-toolset`.

## Proposal

The migration is organized into the following areas, each of which can be implemented and
reviewed independently.

### Phase 1: Base Image and Label Changes

#### 1.1 Operator Dockerfile

**File:** `Dockerfile`

| Line | Current | New |
|------|---------|-----|
| 1 | `registry.access.redhat.com/ubi9/go-toolset:1.26.7` | `registry.access.redhat.com/ubi10/go-toolset:1.26.7` |
| 2 | `registry.access.redhat.com/ubi9/ubi-minimal:latest` | `registry.access.redhat.com/ubi10/ubi-minimal:latest` |
| 52 | `cpe:/a:redhat:multiarch_tuning_operator:1.3::el9` | `cpe:/a:redhat:multiarch_tuning_operator:1.3::el10` |

**Reasoning:** UBI 10 images are GA and ship Go 1.26.7 (confirmed in
`registry.access.redhat.com/ubi10/go-toolset` tags). The runtime image
`ubi10/ubi-minimal` provides glibc 2.39 and kernel headers for 6.12. The `gpgme-devel`
package (required for CGO compilation of the `containers/image` library) is available via
the same `dnf install` command — the package name is unchanged on RHEL 10.

**CGO_CFLAGS consideration:** RHEL 10's GCC defaults to `-march=x86-64-v3`, which emits
instructions that may be incompatible with the ISA-level gate on older nodes. For FIPS
builds using `GOEXPERIMENT=strictfipsruntime`, the CGO wrapper may emit instructions that
fail the `check-isa-level` verification. The fix is to explicitly set
`CGO_CFLAGS=-march=x86-64-v2` in the build step. This ensures binaries remain compatible
with the full x86-64 node fleet. This has been confirmed as a real issue by the OpenShift
Lightspeed Agentic Operator migration
([PR #549](https://github.com/openshift/lightspeed-agentic-operator/pull/549)).

#### 1.2 Makefile

**File:** `Makefile`

| Line | Current | New |
|------|---------|-----|
| 92 | `registry.access.redhat.com/ubi9/go-toolset:1.26` | `registry.access.redhat.com/ubi10/go-toolset:1.26` |
| 93 | `registry.access.redhat.com/ubi9/ubi-minimal:latest` | `registry.access.redhat.com/ubi10/ubi-minimal:latest` |

**Reasoning:** These are the default images for local and containerized builds
(`make test`, `make build`). They must match the Dockerfile base images.

#### 1.3 Bundle Dockerfile

**File:** `bundle.Dockerfile`

| Line | Current | New |
|------|---------|-----|
| 29 | `cpe:/a:redhat:multiarch_tuning_operator:1.3::el9` | `cpe:/a:redhat:multiarch_tuning_operator:1.3::el10` |

**Reasoning:** The bundle image is `FROM scratch` (no RHEL runtime dependency), but the
CPE label must match the operator image's RHEL version for consistent Product Security
tracking.

#### 1.4 Index Dockerfile

**File:** `index.Dockerfile`

| Line | Current | New |
|------|---------|-----|
| 9 | `registry.redhat.io/openshift4/ose-operator-registry-rhel9:v4.16` | `registry.redhat.io/openshift4/ose-operator-registry-rhel10:v<version>` |

**Reasoning:** The OPM server image that serves the FBC (File-Based Catalog) should match
the target RHEL version. The exact tag version depends on which OCP release stream this
targets.

**Open question:** Is `index.Dockerfile` still actively used, or has FBC serving fully
moved to Konflux pipelines? If it's deprecated, it can be removed rather than updated.

### Phase 2: CI Configuration

#### 2.1 CI Operator Config

**File:** `.ci-operator.yaml`

| Line | Current | New |
|------|---------|-----|
| 4 | `rhel-9-golang-1.26-openshift-5.0` | `rhel-10-golang-1.26-openshift-5.0` |

**Reasoning:** This tells Prow/ci-operator which builder image to use as the build root.
The `rhel-9` prefix selects the RHEL 9 builder; `rhel-10` selects the RHEL 10 equivalent.

**Prerequisite:** The `rhel-10-golang-1.26-openshift-5.0` builder image must exist in the
OpenShift CI image pool (`registry.ci.openshift.org/ocp/builder`). As of this writing,
RHEL 10 base images (`base-rhel10`) exist in the `ocp` namespace for OCP 4.23/5.0+
streams, but a `rhel-10-golang-*` builder tag has not been confirmed. This is a **hard
prerequisite** — if the image doesn't exist, it must be requested via ART.

**Additional CI config:** If MTO is onboarded in the `openshift/release` repository,
there may be additional ci-operator config files (e.g.,
`ci-operator/config/openshift/multiarch-tuning-operator/`) that also reference `rhel-9`
builders and need corresponding updates.

### Phase 3: Konflux Pipeline Changes

#### 3.1 Konflux Operator Dockerfile

**File:** `konflux.Dockerfile` (on the `downstream/konflux/references/main` branch)

| Line | Current | New |
|------|---------|-----|
| 2 | `brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.26` | See options below |
| name label | `multiarch-tuning/multiarch-tuning-rhel9-operator` | `multiarch-tuning/multiarch-tuning-rhel10-operator` |
| CPE label | `::el9` | `::el10` |
| runtime | `registry.redhat.io/ubi9/ubi-minimal:latest` | `registry.redhat.io/ubi10/ubi-minimal:latest` |

For the builder image, there are two options depending on timing:

**Option A (if available):** Use the new `registry.redhat.io` location:
```dockerfile
FROM registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel10 as builder
```

**Option B (interim):** Use `registry.access.redhat.com/ubi10/go-toolset:1.26` directly,
which is confirmed available and is what several other operators have adopted:
```dockerfile
FROM registry.access.redhat.com/ubi10/go-toolset:1.26 as builder
```

**Reasoning:** The Brew-hosted `openshift-golang-builder` images are being deprecated by
October 15, 2026 (ART-14300/ART-20920). No `rhel_10` variant of the old Brew path has
been confirmed. Other operators (OpenPerOuter, RHDH must-gather) are already using
`ubi10/go-toolset` or `rhel10/go-toolset` directly in their Konflux Dockerfiles. Option B
is the safer near-term path.

**FIPS build flags:** The current `konflux.Dockerfile` sets
`ENV GOEXPERIMENT=strictfipsruntime`. On RHEL 10, Go 1.26+ supports **native FIPS 140-3**
via `crypto/internal/fips140` instead of the OpenSSL backend. The emerging pattern across
OpenShift is:

```dockerfile
ENV GOEXPERIMENT=strictfipsruntime
ENV GOFIPS140=v1.26.0
ENV CGO_CFLAGS=-march=x86-64-v2
```

- `GOEXPERIMENT=strictfipsruntime` — kept for fail-closed startup checks.
- `GOFIPS140=v1.26.0` — embeds the native Go FIPS module.
- `CGO_CFLAGS=-march=x86-64-v2` — prevents ISA-level incompatibility from RHEL 10's
  default `-march=x86-64-v3`.

#### 3.2 Konflux Bundle Dockerfile

**File:** `bundle.konflux.Dockerfile` (on the `downstream/konflux/references/main` branch)

The builder stage also references `openshift-golang-builder:rhel_9_1.26` and must be
updated the same way as the operator Dockerfile. The CPE label must change to `::el10`.

#### 3.3 Tekton Pipelines

**Files:** `.tekton/*.yaml` (on the `downstream/konflux/references/main` branch)

The pipeline definitions reference `konflux.Dockerfile` by name (not by image), so they
do not need changes for the base image migration itself. However:

- If the Konflux tenant creates a **new component** for the RHEL 10 build, the pipeline
  annotations (`appstudio.openshift.io/component`) must reference the new component name.
- The `build-platforms` parameter (amd64, arm64, ppc64le, s390x) is unchanged.
- The `prefetch-input` for hermetic builds (`gomod-vendor-check`) is unchanged.

#### 3.4 Konflux Tenant Configuration

This is not in the repository — it's in the Konflux tenant config. The migration requires:

1. **Component registration:** Either create a new Konflux component for RHEL 10 or
   update the existing `multiarch-tuning-operator` component to point at the updated
   Dockerfiles.
2. **Errata/Brew component:** Register `multiarch-tuning-rhel10-operator` in Brew/Errata
   (or confirm the naming convention with the release team).
3. **ImageDigestMirrorSet:** Update the source reference in
   `deploy/base/.../imagedigestmirrorset.yaml` from
   `multiarch-tuning-rhel9-operator` to `multiarch-tuning-rhel10-operator`.

### Phase 4: Tooling and Automation Updates

#### 4.1 Version Bump Script

**File:** `hack/bump-version.sh`

| Lines | Current | New |
|-------|---------|-----|
| 31, 32, 38, 39 | `::el9` (hardcoded in sed patterns) | `::el10` |

**Reasoning:** This script updates CPE labels when bumping the operator version. The
`el9` suffix is hardcoded in both the macOS (BSD sed) and Linux (GNU sed) code paths.

#### 4.2 Bundle Dockerfile Patch Script

**File:** `hack/patch-bundle-dockerfile.sh`

| Line | Current | New |
|------|---------|-----|
| 14 | `::el9` (hardcoded in CONTENT variable) | `::el10` |

**Reasoning:** This script appends Red Hat labels (including the CPE label) to
`bundle.Dockerfile` during the `make bundle` process.

#### 4.3 Upgrade Automation Scripts

**File:** `hack/upgrade-automation/scripts/lib/file-updates.sh`

Five functions contain hardcoded RHEL 9 patterns:

| Function | Line | Current Pattern | New Pattern |
|----------|------|-----------------|-------------|
| `update_ci_operator_yaml()` | 32 | `rhel-9-golang-` | `rhel-10-golang-` |
| `update_tekton_files()` | 46 | `rhel_9_` | `rhel_10_` (or new registry path) |
| `update_makefile_build_image()` | 72 | `rhel-9-golang-` | `rhel-10-golang-` (or new registry path) |
| `update_bundle_konflux_dockerfile()` | 84 | `rhel_9_` | `rhel_10_` (or new registry path) |
| `update_konflux_dockerfile()` | 98 | `rhel_9_` | `rhel_10_` (or new registry path) |

**Reasoning:** These scripts automate Go version and OCP version bumps. The sed patterns
match on `rhel-9` / `rhel_9_` prefixes to find and replace image tags. If the RHEL
version in the pattern doesn't match the actual Dockerfile content, the regex silently
fails — a silent failure that is easy to miss.

**Note:** If the Konflux Dockerfiles migrate from Brew (`openshift-golang-builder`) to
`registry.access.redhat.com/ubi10/go-toolset`, the sed patterns in
`update_tekton_files()`, `update_bundle_konflux_dockerfile()`, and
`update_konflux_dockerfile()` must be rewritten entirely to match the new image reference
format.

#### 4.4 Image Digest Mirror Set

**File:** `deploy/base/config.openshift.io/imagedistedmirrorsets/multiarch-tuning-operator-fbc-staging/imagedigestmirrorset.yaml`

| Line | Current | New |
|------|---------|-----|
| 12 | `registry.redhat.io/multiarch-tuning/multiarch-tuning-rhel9-operator` | `registry.redhat.io/multiarch-tuning/multiarch-tuning-rhel10-operator` |

**Reasoning:** The IDMS maps the production registry image to the Konflux staging image.
The source name must match the Brew/Errata component name.

### Phase 5: Test Fixture Updates

#### 5.1 Example Registry Auth Test Manifest

**File:** `test/manifests/06-example-registry-auth.yaml`

| Line | Current | New |
|------|---------|-----|
| 16 | `registry.redhat.io/rhel8/httpd-24:latest` | A RHEL 10 or UBI 10 based image |

**Reasoning:** This is a test fixture image, not a shipped component. It currently
references a RHEL 8 image. While this won't break the RHEL 10 migration, updating it
ensures test fixtures don't pull from an EOL RHEL version.

## Implementation Details/Notes/Constraints

### CGO and gpgme-devel on RHEL 10

The operator has a hard CGO dependency on `gpgme-devel` through the dependency chain:
`containers/image/v5` → `proglottis/gpgme` → system `libgpgme`.

On RHEL 9, `gpgme-devel` is available in the CodeReady Builder (CRB) repository. On
RHEL 10, the equivalent repository is `codeready-builder-for-ubi-10-x86_64-rpms`. The
package name is unchanged. The `gpgme` C API is stable across RHEL versions, so the
vendored Go bindings (`proglottis/gpgme`) should compile without modification.

**Verification required:** Before merging, the build must be tested to confirm:
1. `dnf install -y gpgme-devel` succeeds in the `ubi10/go-toolset` builder image.
2. The resulting binary links correctly against `libgpgme.so` on `ubi10/ubi-minimal`.
3. `ldd manager | grep gpgme` shows the expected shared library on the runtime image.

### RHEL 10 x86-64-v3 Default

RHEL 10's GCC defaults to `-march=x86-64-v3`, which requires AVX2 support. Not all nodes
in a multi-architecture cluster may support this. The `CGO_CFLAGS=-march=x86-64-v2` flag
is necessary to ensure the compiled binary runs on the full x86-64 fleet. This applies to
both the upstream Dockerfile and the Konflux Dockerfile.

### Brew golang-builder Deprecation

The `brew.registry.redhat.io/rh-osbs/openshift-golang-builder` images are deprecated as
of ART-14300, with a deadline of **October 15, 2026**. The new location is
`registry.redhat.io/openshift/golang-builder` with tags like
`golang-builder-v1.26-rhel9`. A `rhel10` variant is expected but not yet confirmed. If
the RHEL 10 variant is not available by the time this enhancement is implemented, use
`registry.access.redhat.com/ubi10/go-toolset:1.26` as the Konflux builder image (this is
what other operators have done).

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `gpgme-devel` not available or ABI-incompatible on UBI 10 | Build failure or runtime crash | Test in UBI 10 container before merging. gpgme API is stable; risk is low. |
| `rhel-10-golang-*` CI builder image doesn't exist yet | CI builds fail | Request the image via ART. Fall back to `ubi10/go-toolset` if needed. |
| RHEL 10 x86-64-v3 default causes ISA-level failures | Binary crashes on older x86-64 nodes | Set `CGO_CFLAGS=-march=x86-64-v2` in all Dockerfiles. |
| FIPS crypto backend change (OpenSSL → native Go) | FIPS certification gap | Set `GOFIPS140=v1.26.0` to embed the native Go FIPS module. Follow OpenShift-wide FIPS guidance. |
| Brew golang-builder `rhel_10` tag doesn't exist | Konflux build fails | Use `ubi10/go-toolset` directly (confirmed available). |

## Drawbacks

- One-time churn across multiple files (Dockerfiles, Makefile, CI config, hack scripts).
  However, the changes are mechanical and low-risk.
- If RHEL 9 and RHEL 10 streams must coexist temporarily (e.g., backport releases), the
  branching strategy adds maintenance burden.

## Test Plan

### Build Verification

- **Local build:** `NO_DOCKER=1 make build` — confirms Go code compiles with RHEL 10
  toolchain.
- **Containerized build:** `make docker-build IMG=<test-registry>/mto:rhel10-test` —
  confirms the full Dockerfile builds successfully on UBI 10 base images.
- **Multi-arch build:** `make docker-buildx IMG=<test-registry>/mto:rhel10-test` —
  confirms amd64, arm64, ppc64le, s390x all build on UBI 10.

### CGO/gpgme Verification

- **Package install:** Run `docker run --rm ubi10/go-toolset:1.26 bash -c "dnf install -y gpgme-devel && pkg-config --exists gpgme && echo OK"` — confirms `gpgme-devel` installs on UBI 10.
- **Shared library check:** Build the operator image and run
  `docker run --rm <image> ldd /manager | grep gpgme` — confirms `libgpgme.so` is
  correctly linked in the runtime image.

### Unit Tests

- `make unit` (or `NO_DOCKER=1 make unit`) — no changes expected. Unit tests use envtest
  and have no RHEL dependency.

### E2E Tests

- `make e2e` against an OCP cluster with RHEL 10 nodes — confirms the operator functions
  correctly on the target platform.
- Specifically verify the image inspection flow (the primary consumer of gpgme) works
  end-to-end: deploy a pod with a multi-arch image and confirm the scheduling gate is
  added and removed correctly.

### FIPS Verification

- Build the Konflux Dockerfile with `GOEXPERIMENT=strictfipsruntime` and
  `GOFIPS140=v1.26.0` on a FIPS-enabled RHEL 10 system.
- Confirm the binary starts without FIPS-related panics.
- Confirm `go version -m manager` shows the FIPS module embedded.

### CI Pipeline Verification

- After updating `.ci-operator.yaml`, trigger a CI run and confirm the build root uses
  the RHEL 10 builder image.
- After updating Konflux Dockerfiles, trigger a Konflux build and confirm the image is
  built and pushed successfully.

## Graduation Criteria

This is a one-time infrastructure migration, not a feature. There is no graduation
process — the migration is complete when all builds, tests, and release pipelines
successfully run on RHEL 10 base images.

## Upgrade / Downgrade Strategy

No special upgrade/downgrade strategy is required. The operator binary is
RHEL-version-agnostic — it runs on whatever base image it's packaged in. Existing
clusters running the RHEL 9-based operator will receive the RHEL 10-based operator
through the normal OLM upgrade channel.

## Version Skew Strategy

Not applicable. The RHEL version of the base image is transparent to the Kubernetes API
server and cluster components.

## Operational Aspects of API Extensions

No API changes are part of this enhancement.

## Implementation History

| Date | Event |
|------|-------|
| 2026-09-30 | Enhancement proposed |

## Alternatives

### Maintain Dual RHEL 9 / RHEL 10 Builds

Maintain separate branches or build configurations for both RHEL versions. Rejected
because it doubles the CI/CD maintenance burden with no user-facing benefit — RHEL 9 and
RHEL 10 operators are functionally identical.

### Wait for Brew golang-builder rhel_10 Tag

Delay the migration until `openshift-golang-builder:rhel_10_*` is published in Brew.
Not recommended because: (a) the Brew path is being deprecated entirely by October 15,
2026, (b) `ubi10/go-toolset` is already available and proven, and (c) other operators have
already migrated without waiting.

## Infrastructure Needed

- Confirmation that `rhel-10-golang-1.26-openshift-5.0` (or equivalent) exists in the
  OpenShift CI image pool. If not, request it via ART.
- Brew/Errata component registration for `multiarch-tuning-rhel10-operator` (or
  confirmation of the naming convention from the release team).
- Konflux tenant component update (new component or in-place update).

## Open Questions

1. **Brew/Errata component naming:** Does `multiarch-tuning-rhel9-operator` become
   `multiarch-tuning-rhel10-operator`, or does Red Hat use a different naming convention
   for the RHEL 10 transition? This must be confirmed with the release team.

2. **Konflux component strategy:** Should a new Konflux application/component be created
   for RHEL 10, or should the existing one be updated in-place? This affects whether the
   downstream branch structure changes.

3. **`index.Dockerfile` status:** Is this file still actively used for FBC serving, or
   has it been superseded by Konflux FBC pipelines? If deprecated, it can be removed.

4. **OpenShift CI builder availability:** Is `rhel-10-golang-1.26-openshift-5.0`
   available in the CI image pool? If not, what is the process and timeline for requesting
   it via ART?

5. **FIPS certification timeline:** Is Go's native FIPS 140-3 module
   (`GOFIPS140=v1.26.0`) approved for use in OpenShift-shipped operators, or is there a
   transition period during which the OpenSSL backend must still be used on RHEL 10?

6. **Branching strategy:** Will RHEL 9 releases continue from a `release-*` branch while
   `main` moves to RHEL 10, or is this a clean cutover with no further RHEL 9 releases?

## References

- [UBI 10 go-toolset](https://catalog.redhat.com/software/containers/ubi10/go-toolset) — GA, ships Go 1.26.7
- [UBI 10 ubi-minimal](https://catalog.redhat.com/software/containers/ubi10/ubi-minimal) — GA, version 10.2
- [ART-14300: Brew golang-builder deprecation](https://issues.redhat.com/browse/ART-14300) — deadline October 15, 2026
- [OpenShift Lightspeed Agentic Operator RHEL 10 migration PR](https://github.com/openshift/lightspeed-agentic-operator/pull/549) — documents CGO_CFLAGS gotcha
- [RHDH must-gather UBI 10 migration PR](https://github.com/redhat-developer/rhdh-must-gather/pull/290) — full migration example
- [OpenShift Builds Operator UBI 10 tracking PR](https://github.com/redhat-openshift-builds/operator/pull/1512) — ongoing migration
