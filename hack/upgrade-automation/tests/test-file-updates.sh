set -uo pipefail

PASS=0
FAIL=0
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
LIB="$REPO_ROOT/hack/upgrade-automation/scripts/lib/file-updates.sh"

# shellcheck source=../scripts/lib/file-updates.sh
source "$LIB"

# The library sets -euo pipefail; the test harness needs to tolerate
# non-zero exits from the functions under test, so disable errexit here.
set +e

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

pass() { ((PASS++)); echo "  PASS: $1"; }
fail() { ((FAIL++)); echo "  FAIL: $1"; }

# Run a function and assert it exits 0.
expect_ok() {
    local description="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        pass "$description"
    else
        fail "$description (expected success, got failure)"
    fi
}

# Run a function and assert it exits non-zero (visible failure).
expect_fail() {
    local description="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        fail "$description (expected failure, got success)"
    else
        pass "$description"
    fi
}

assert_contains() {
    local file="$1" pattern="$2" description="$3"
    if grep -qE "$pattern" "$file"; then
        pass "$description"
    else
        fail "$description (pattern '$pattern' not found in $file)"
    fi
}

assert_not_contains() {
    local file="$1" pattern="$2" description="$3"
    if grep -qE "$pattern" "$file"; then
        fail "$description (unexpected pattern '$pattern' found in $file)"
    else
        pass "$description"
    fi
}

TARGET_VERSION="1.26"
OLD_VERSION="1.24"

echo "═══════════════════════════════════════════════════════════════"
echo " file-updates.sh tests"
echo "═══════════════════════════════════════════════════════════════"

# ── update_dockerfile ───────────────────────────────────────────
echo ""
echo "── update_dockerfile ──"

mkdir -p "$TMPDIR/dockerfile-ok" && cd "$TMPDIR/dockerfile-ok"
cat > Dockerfile <<'EOF'
ARG BUILD_IMAGE=registry.access.redhat.com/ubi9/go-toolset:1.24
ARG RUNTIME_IMAGE=registry.access.redhat.com/ubi9/ubi-minimal:latest
FROM ${BUILD_IMAGE} as builder
EOF
expect_ok "update_dockerfile succeeds on current layout" update_dockerfile "$TARGET_VERSION"
assert_contains "Dockerfile" "ARG BUILD_IMAGE=registry\.access\.redhat\.com/ubi9/go-toolset:${TARGET_VERSION}([^0-9.]|\$)" \
    "update_dockerfile bumps ARG BUILD_IMAGE go-toolset version"
assert_not_contains "Dockerfile" "go-toolset:${OLD_VERSION}" \
    "update_dockerfile leaves no stale go-toolset version"

# Idempotence: re-running with the same version must not fail
expect_ok "update_dockerfile is idempotent" update_dockerfile "$TARGET_VERSION"

# Missing target: Dockerfile without the BUILD_IMAGE arg must fail visibly
mkdir -p "$TMPDIR/dockerfile-broken" && cd "$TMPDIR/dockerfile-broken"
cat > Dockerfile <<'EOF'
FROM some-unknown-builder:latest
EOF
expect_fail "update_dockerfile fails when BUILD_IMAGE arg is missing" update_dockerfile "$TARGET_VERSION"

# ── update_makefile_build_image ─────────────────────────────────
echo ""
echo "── update_makefile_build_image ──"

mkdir -p "$TMPDIR/makefile-ok" && cd "$TMPDIR/makefile-ok"
cat > Makefile <<'EOF'
BUILD_IMAGE ?= registry.access.redhat.com/ubi9/go-toolset:1.24
EOF
expect_ok "update_makefile_build_image succeeds on current layout" update_makefile_build_image "$TARGET_VERSION" "5.0"
assert_contains "Makefile" "BUILD_IMAGE \?= registry\.access\.redhat\.com/ubi9/go-toolset:${TARGET_VERSION}([^0-9.]|\$)" \
    "update_makefile_build_image bumps BUILD_IMAGE go-toolset version"

# Legacy layout (registry.ci.openshift.org OCP builder) must still update
mkdir -p "$TMPDIR/makefile-legacy" && cd "$TMPDIR/makefile-legacy"
cat > Makefile <<'EOF'
BUILD_IMAGE ?= registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.24-openshift-4.19
EOF
expect_ok "update_makefile_build_image succeeds on legacy layout" update_makefile_build_image "$TARGET_VERSION" "5.0"
assert_contains "Makefile" "rhel-9-golang-${TARGET_VERSION}-openshift-5\.0" \
    "update_makefile_build_image bumps legacy OCP builder image"

# Missing target: Makefile without BUILD_IMAGE must fail visibly
mkdir -p "$TMPDIR/makefile-broken" && cd "$TMPDIR/makefile-broken"
echo "IMG ?= example" > Makefile
expect_fail "update_makefile_build_image fails when BUILD_IMAGE is missing" update_makefile_build_image "$TARGET_VERSION" "5.0"

# ── update_tekton_files ─────────────────────────────────────────
echo ""
echo "── update_tekton_files ──"

mkdir -p "$TMPDIR/tekton-ok/.tekton" && cd "$TMPDIR/tekton-ok"
cat > .tekton/pipeline.yaml <<'EOF'
    - name: SCRIPT_RUNNER_IMAGE
      value: registry.access.redhat.com/ubi9/go-toolset:1.24
EOF
cat > .tekton/other.yaml <<'EOF'
image: brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.24
EOF
expect_ok "update_tekton_files succeeds on current layout" update_tekton_files "$TARGET_VERSION"
assert_contains ".tekton/pipeline.yaml" "ubi9/go-toolset:${TARGET_VERSION}([^0-9.]|\$)" \
    "update_tekton_files bumps go-toolset script runner image"
assert_not_contains ".tekton/other.yaml" "rhel_9_${OLD_VERSION}" \
    "update_tekton_files bumps Brew Go builder references"

# Missing target: no go-toolset reference at all must fail visibly
mkdir -p "$TMPDIR/tekton-broken/.tekton" && cd "$TMPDIR/tekton-broken"
echo "foo: bar" > .tekton/pipeline.yaml
expect_fail "update_tekton_files fails when no go-toolset reference exists" update_tekton_files "$TARGET_VERSION"

# ── update_ci_operator_yaml ─────────────────────────────────────
echo ""
echo "── update_ci_operator_yaml ──"

mkdir -p "$TMPDIR/ci-ok" && cd "$TMPDIR/ci-ok"
cat > .ci-operator.yaml <<'EOF'
build_root_image:
  name: builder
  namespace: ocp
  tag: rhel-9-golang-1.24-openshift-4.19
EOF
expect_ok "update_ci_operator_yaml succeeds on current layout" update_ci_operator_yaml "$TARGET_VERSION" "5.0"
assert_contains ".ci-operator.yaml" "rhel-9-golang-${TARGET_VERSION}-openshift-5\.0" \
    "update_ci_operator_yaml bumps build root tag"

mkdir -p "$TMPDIR/ci-broken" && cd "$TMPDIR/ci-broken"
echo "build_root_image: {}" > .ci-operator.yaml
expect_fail "update_ci_operator_yaml fails when build root tag is missing" update_ci_operator_yaml "$TARGET_VERSION" "5.0"

# ── update_bundle_dockerfile_builder ────────────────────────────
echo ""
echo "── update_bundle_dockerfile_builder ──"

mkdir -p "$TMPDIR/bundle-ok" && cd "$TMPDIR/bundle-ok"
cat > bundle.Dockerfile <<'EOF'
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 AS builder
EOF
expect_ok "update_bundle_dockerfile_builder succeeds on current layout" update_bundle_dockerfile_builder "$TARGET_VERSION"
assert_contains "bundle.Dockerfile" "ubi9/go-toolset:${TARGET_VERSION}([^0-9.]|\$)" \
    "update_bundle_dockerfile_builder bumps builder image"

# Missing file: skipped without failure (bundle is optional)
mkdir -p "$TMPDIR/bundle-missing" && cd "$TMPDIR/bundle-missing"
expect_ok "update_bundle_dockerfile_builder skips when file is missing" update_bundle_dockerfile_builder "$TARGET_VERSION"

# Present but mismatched: must fail visibly
mkdir -p "$TMPDIR/bundle-broken" && cd "$TMPDIR/bundle-broken"
echo "FROM totally-different-image:latest AS builder" > bundle.Dockerfile
expect_fail "update_bundle_dockerfile_builder fails when builder image is unrecognized" update_bundle_dockerfile_builder "$TARGET_VERSION"

# ── update_rpms_in_yaml ─────────────────────────────────────────
echo ""
echo "── update_rpms_in_yaml ──"

mkdir -p "$TMPDIR/rpms-ok" && cd "$TMPDIR/rpms-ok"
cat > rpms.in.yaml <<EOF
packages:
  - gpgme-devel
arches: [x86_64, aarch64, ppc64le, s390x]
context:
  image: registry.access.redhat.com/ubi9/go-toolset:${OLD_VERSION}
contentOrigin:
  repofiles:
    - ./ubi9.repo
EOF
expect_ok "update_rpms_in_yaml succeeds on current layout" update_rpms_in_yaml "$TARGET_VERSION"
assert_contains "rpms.in.yaml" "ubi9/go-toolset:${TARGET_VERSION}([^0-9.]|\$)" \
    "update_rpms_in_yaml bumps resolution context image"

# Present but mismatched: must fail visibly
mkdir -p "$TMPDIR/rpms-broken" && cd "$TMPDIR/rpms-broken"
echo "packages: []" > rpms.in.yaml
expect_fail "update_rpms_in_yaml fails when context image is missing" update_rpms_in_yaml "$TARGET_VERSION"

# Missing file: skipped without failure
mkdir -p "$TMPDIR/rpms-missing" && cd "$TMPDIR/rpms-missing"
expect_ok "update_rpms_in_yaml skips when file is missing" update_rpms_in_yaml "$TARGET_VERSION"

# ── update_rpms_lockfile ────────────────────────────────────────
echo ""
echo "── update_rpms_lockfile ──"

# Missing rpms.in.yaml: skipped without failure
mkdir -p "$TMPDIR/lockfile-missing" && cd "$TMPDIR/lockfile-missing"
expect_ok "update_rpms_lockfile skips when rpms.in.yaml is missing" update_rpms_lockfile

# rpms.in.yaml present but tool unavailable: must fail visibly
mkdir -p "$TMPDIR/lockfile-notool" && cd "$TMPDIR/lockfile-notool"
echo "packages: []" > rpms.in.yaml
if command -v rpm-lockfile-prototype >/dev/null 2>&1; then
    echo "  SKIP: update_rpms_lockfile tool-missing failure (rpm-lockfile-prototype installed on this machine)"
else
    expect_fail "update_rpms_lockfile fails when the tool is unavailable" update_rpms_lockfile
fi

# ── verify_contains ─────────────────────────────────────────────
echo ""
echo "── verify_contains ──"

mkdir -p "$TMPDIR/verify" && cd "$TMPDIR/verify"
echo "hello world" > sample.txt
expect_ok "verify_contains passes on matching pattern" verify_contains "sample.txt" "^hello" "sample greeting"
expect_fail "verify_contains fails on non-matching pattern" verify_contains "sample.txt" "^goodbye" "sample greeting"
expect_fail "verify_contains fails on missing file" verify_contains "nope.txt" "." "missing file"

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo " Results: $PASS passed, $FAIL failed"
echo "═══════════════════════════════════════════════════════════════"

if (( FAIL > 0 )); then
    exit 1
fi
exit 0
