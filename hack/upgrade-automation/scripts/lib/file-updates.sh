#!/bin/bash
# File update functions for multiarch-tuning-operator upgrades
#
# This library provides functions to update specific files during version upgrades.
# All regex patterns and file locations are operator-specific.

set -euo pipefail

# Portable sed in-place function that works on both GNU sed (Linux) and BSD sed (macOS)
# Usage: sed_inplace 's/pattern/replacement/' file
sed_inplace() {
    local pattern="$1"
    local file="$2"

    # Detect OS and use appropriate sed syntax
    if sed --version >/dev/null 2>&1; then
        # GNU sed (Linux)
        sed -i "$pattern" "$file"
    else
        # BSD sed (macOS)
        sed -i '' "$pattern" "$file"
    fi
}

verify_contains() {
    local file="$1"
    local pattern="$2"
    local description="$3"

    if [[ ! -f "$file" ]]; then
        echo "❌ ERROR: $description: file not found: $file" >&2
        return 1
    fi

    if ! grep -qE "$pattern" "$file"; then
        echo "❌ ERROR: $description: expected '$pattern' not found in $file" >&2
        echo "   The file layout may have changed; update the upgrade automation" >&2
        echo "   in hack/upgrade-automation/scripts/lib/file-updates.sh accordingly." >&2
        return 1
    fi

    return 0
}

# Update .ci-operator.yaml with Go and OCP versions
update_ci_operator_yaml() {
    local go_version="$1"
    local ocp_version="$2"

    echo "Updating .ci-operator.yaml..." >&2

    if [[ ! -f .ci-operator.yaml ]]; then
        echo "❌ ERROR: .ci-operator.yaml not found" >&2
        return 1
    fi

    sed_inplace "s/\(tag:\s\+rhel-9-golang-\)\([0-9]\+\.[0-9]\+\)\(\(-builder-multi\)\?-openshift-\)\([0-9]\+\.[0-9]\+\)/\1${go_version}\3${ocp_version}/" .ci-operator.yaml

    verify_contains ".ci-operator.yaml" "rhel-9-golang-${go_version}(-builder-multi)?-openshift-${ocp_version}" \
        ".ci-operator.yaml build root" || return 1

    echo "  ✅ Updated .ci-operator.yaml" >&2
}

# Update .tekton YAML files with Go version
# Covers both ubi9/go-toolset references and the legacy Brew Go builder references.
update_tekton_files() {
    local go_version="$1"

    echo "Updating .tekton/*.yaml files..." >&2

    if [[ ! -d .tekton ]]; then
        echo "  ℹ️  No .tekton directory found, skipping" >&2
        return 0
    fi

    # Update each .tekton YAML file using portable sed
    while IFS= read -r -d '' file; do
        sed_inplace "s/\(brew\.registry\.redhat\.io\/rh-osbs\/openshift-golang-builder:rhel_9_\)\([0-9]\+\.[0-9]\+\)/\1${go_version}/" "$file"
        sed_inplace "s/\(registry\.access\.redhat\.com\/ubi9\/go-toolset:\)\([0-9]\+\.[0-9]\+\)/\1${go_version}/" "$file"
    done < <(find .tekton -name '*.yaml' -type f -print0)

    # Fail visibly if the expected references are missing or still stale.
    if ! grep -rqE "ubi9/go-toolset:${go_version}([^0-9.]|\$)" .tekton; then
        echo "❌ ERROR: no ubi9/go-toolset:${go_version} reference found in .tekton/ after update" >&2
        echo "   Expected at least one go-toolset reference (e.g. the SCRIPT_RUNNER_IMAGE" >&2
        echo "   of the build pipelines) to be updated to Go ${go_version}." >&2
        return 1
    fi
    local stale
    stale=$(grep -rE "ubi9/go-toolset:[0-9]+\.[0-9]+" .tekton | grep -vE "ubi9/go-toolset:${go_version}([^0-9.]|\$)" || true)
    if [[ -n "$stale" ]]; then
        echo "❌ ERROR: stale go-toolset references remain in .tekton/ after update:" >&2
        echo "$stale" >&2
        return 1
    fi
    local stale_brew
    stale_brew=$(grep -rE "openshift-golang-builder:rhel_9_[0-9]+\.[0-9]+" .tekton | grep -v "rhel_9_${go_version}" || true)
    if [[ -n "$stale_brew" ]]; then
        echo "❌ ERROR: stale Brew Go builder references remain in .tekton/ after update:" >&2
        echo "$stale_brew" >&2
        return 1
    fi

    echo "  ✅ Updated .tekton files" >&2
}

# Update Dockerfile with Go version
# The builder is selected via the BUILD_IMAGE argument, defaulting to ubi9/go-toolset
# (Red Hat toolchain, required for GOEXPERIMENT=strictfipsruntime).
update_dockerfile() {
    local go_version="$1"

    echo "Updating Dockerfile..." >&2

    if [[ ! -f Dockerfile ]]; then
        echo "❌ ERROR: Dockerfile not found" >&2
        return 1
    fi

    sed_inplace "s/\(FROM\s\+golang:\)\([0-9]\+\.[0-9]\+\)/\1${go_version}/" Dockerfile
    sed_inplace "s/\(ARG BUILD_IMAGE=registry\.access\.redhat\.com\/ubi9\/go-toolset:\)\([0-9]\+\.[0-9]\+\)/\1${go_version}/" Dockerfile

    verify_contains "Dockerfile" "ubi9/go-toolset:${go_version}([^0-9.]|\$)" \
        "Dockerfile builder image (ARG BUILD_IMAGE)" || return 1

    echo "  ✅ Updated Dockerfile" >&2
}

# Update Makefile BUILD_IMAGE with Go version
update_makefile_build_image() {
    local go_version="$1"
    local ocp_version="$2"

    echo "Updating Makefile BUILD_IMAGE..." >&2

    if [[ ! -f Makefile ]]; then
        echo "❌ ERROR: Makefile not found" >&2
        return 1
    fi

    # Current layout: BUILD_IMAGE ?= registry.access.redhat.com/ubi9/go-toolset:X.Y
    # Legacy layout (kept for robustness): registry.ci.openshift.org/ocp/builder:rhel-9-golang-X.Y(-builder-multi)?-openshift-Z.Z
    sed_inplace "s/\(BUILD_IMAGE\s*?=\s*registry\.access\.redhat\.com\/ubi9\/go-toolset:\)\([0-9]\+\.[0-9]\+\)/\1${go_version}/" Makefile
    sed_inplace "s/\(BUILD_IMAGE\s*?=\s*registry\.ci\.openshift\.org\/ocp\/builder:rhel-9-golang-\)\([0-9]\+\.[0-9]\+\)\(\(-builder-multi\)\?-openshift-\)\([0-9]\+\.[0-9]\+\)/\1${go_version}\3${ocp_version}/" Makefile

    verify_contains "Makefile" "(ubi9/go-toolset:${go_version}([^0-9.]|\$)|rhel-9-golang-${go_version}(-builder-multi)?-openshift-${ocp_version})" \
        "Makefile BUILD_IMAGE" || return 1

    echo "  ✅ Updated Makefile BUILD_IMAGE" >&2
}

# Update bundle.Dockerfile builder stage with go-toolset version
update_bundle_dockerfile_builder() {
    local go_version="$1"

    echo "Updating bundle.Dockerfile builder image..." >&2

    if [[ ! -f bundle.Dockerfile ]]; then
        echo "  ℹ️  bundle.Dockerfile not found, skipping" >&2
        return 0
    fi

    # Update go-toolset version in the builder stage if present
    sed_inplace "s/\(FROM\s\+registry\.access\.redhat\.com\/ubi9\/go-toolset:\)\([^ ]\+\)/\1${go_version}/" bundle.Dockerfile

    verify_contains "bundle.Dockerfile" "ubi9/go-toolset:${go_version}([^0-9.]|\$)" \
        "bundle.Dockerfile builder image" || return 1

    echo "  ✅ Updated bundle.Dockerfile builder image" >&2
}

# Update konflux.Dockerfile with Go version
# Legacy file (Konflux builds use Dockerfile since MULTIARCH-6532); update as long as it exists.
update_konflux_dockerfile() {
    local go_version="$1"

    echo "Updating konflux.Dockerfile..." >&2

    if [[ ! -f konflux.Dockerfile ]]; then
        echo "  ℹ️  konflux.Dockerfile not found, skipping" >&2
        return 0
    fi

    sed_inplace "s/\(FROM\s\+brew\.registry\.redhat\.io\/rh-osbs\/openshift-golang-builder:rhel_9_\)\([0-9]\+\.[0-9]\+\)/\1${go_version}/" konflux.Dockerfile

    verify_contains "konflux.Dockerfile" "openshift-golang-builder:rhel_9_${go_version}([^0-9.]|\$)" \
        "konflux.Dockerfile builder image" || return 1

    echo "  ✅ Updated konflux.Dockerfile" >&2
}

# Update rpms.in.yaml with the Go version of the builder image used as the
# resolution context. The lockfile must be regenerated afterwards (the packages
# already installed in the builder image may change between Go versions).
update_rpms_in_yaml() {
    local go_version="$1"

    echo "Updating rpms.in.yaml builder image..." >&2

    if [[ ! -f rpms.in.yaml ]]; then
        echo "  ℹ️  rpms.in.yaml not found, skipping" >&2
        return 0
    fi

    sed_inplace "s/\(image:\s\+registry\.access\.redhat\.com\/ubi9\/go-toolset:\)\([0-9]\+\.[0-9]\+\)/\1${go_version}/" rpms.in.yaml

    verify_contains "rpms.in.yaml" "ubi9/go-toolset:${go_version}([^0-9.]|\$)" \
        "rpms.in.yaml resolution context image" || return 1

    echo "  ✅ Updated rpms.in.yaml builder image" >&2
}

# Regenerate rpms.lock.yaml with rpm-lockfile-prototype.
update_rpms_lockfile() {
    echo "Regenerating rpms.lock.yaml..." >&2

    if [[ ! -f rpms.in.yaml ]]; then
        echo "  ℹ️  rpms.in.yaml not found, skipping" >&2
        return 0
    fi

    if ! command -v rpm-lockfile-prototype >/dev/null 2>&1; then
        echo "⚠️  rpm-lockfile-prototype not installed; cannot regenerate rpms.lock.yaml" >&2
        echo "   The builder image in rpms.in.yaml was updated, so the lockfile MUST be" >&2
        echo "   regenerated before merging. Install the tool and run:" >&2
        echo "     make rpm-lockfile" >&2
        return 1
    fi

    rpm-lockfile-prototype rpms.in.yaml

    verify_contains "rpms.lock.yaml" "^lockfileVersion:" \
        "rpms.lock.yaml regeneration" || return 1

    echo "  ✅ Regenerated rpms.lock.yaml" >&2
}

# Update go.mod go directive
update_go_mod_directive() {
    local go_version="$1"

    # go.mod supports either X.Y or X.Y.Z format
    # Use full version if it has a patch, otherwise use X.Y
    local go_mod_version
    if [[ "$go_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        # Has patch version (e.g., 1.24.11)
        go_mod_version="$go_version"
    else
        # Only major.minor (e.g., 1.24)
        go_mod_version="$go_version"
    fi

    echo "Updating go.mod go directive..." >&2

    # Match and replace any go version (X.Y or X.Y.Z or X.Y.Z.W)
    sed_inplace "s/^go [0-9]\+\.[0-9]\+\(\.[0-9]\+\)*$/go ${go_mod_version}/" go.mod

    echo "  ✅ Updated go directive to $go_mod_version" >&2
}

# Update Makefile tool versions
update_makefile_tool_versions() {
    local kustomize_version="$1"
    local controller_tools_version="$2"
    local setup_envtest_version="$3"
    local envtest_k8s_version="$4"
    local golint_version="$5"

    echo "Updating Makefile tool versions..." >&2

    sed_inplace "s/\(KUSTOMIZE_VERSION\s*?=\s*\)v[0-9]\+\.[0-9]\+\.[0-9]\+/\1${kustomize_version}/" Makefile
    sed_inplace "s/\(CONTROLLER_TOOLS_VERSION\s*?=\s*\)v[0-9]\+\.[0-9]\+\.[0-9]\+/\1${controller_tools_version}/" Makefile
    sed_inplace "s/\(SETUP_ENVTEST_VERSION\s*?=\s*\)release-[0-9]\+\.[0-9]\+/\1${setup_envtest_version}/" Makefile
    sed_inplace "s/\(ENVTEST_K8S_VERSION\s*=\s*\)[0-9v]\+\.[0-9]\+\.[0-9]\+/\1${envtest_k8s_version}/" Makefile
    sed_inplace "s/\(GOLINT_VERSION\s*=\s*\)v[0-9]\+\.[0-9]\+\.[0-9]\+/\1${golint_version}/" Makefile

    echo "  ✅ KUSTOMIZE_VERSION = $kustomize_version" >&2
    echo "  ✅ CONTROLLER_TOOLS_VERSION = $controller_tools_version" >&2
    echo "  ✅ SETUP_ENVTEST_VERSION = $setup_envtest_version" >&2
    echo "  ✅ ENVTEST_K8S_VERSION = $envtest_k8s_version" >&2
    echo "  ✅ GOLINT_VERSION = $golint_version" >&2
}
