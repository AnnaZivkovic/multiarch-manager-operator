#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(dirname "$script_dir")"
sdk="${OPERATOR_SDK:-$repo_dir/bin/operator-sdk}"
kustomize="${KUSTOMIZE:-$repo_dir/bin/kustomize}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

cp -r "$repo_dir/config" "$repo_dir/bundle" "$repo_dir/PROJECT" "$tmp_dir/"
cp "$repo_dir/bundle.Dockerfile" "$tmp_dir/bundle.Dockerfile"
cp "$script_dir/patch-bundle-dockerfile.sh" "$tmp_dir/"
cd "$tmp_dir"

# Konflux may nudge both the builder base image and the operator digest.
sed -e '1c\FROM registry.example.com/builder@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa as builder' \
    -e 's|^ARG IMG=.*|ARG IMG=registry.example.com/operator@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb|' \
    bundle.Dockerfile > saved.Dockerfile
cp saved.Dockerfile bundle.Dockerfile
sed '/^FROM scratch/,$d' saved.Dockerfile > expected-builder

# Catch digest loss at the actual SDK overwrite boundary, not only direct patching.
"$kustomize" build config/manifests | "$sdk" generate bundle -q --overwrite \
    --version 1.4.0 --channels stable --default-channel stable
VERSION=1.4.0 ./patch-bundle-dockerfile.sh saved.Dockerfile
sed '/^FROM scratch/,$d' bundle.Dockerfile > actual-builder
diff -u expected-builder actual-builder
[[ "$(grep -c '^FROM .* as builder$' bundle.Dockerfile)" -eq 1 ]]
grep -qFx 'COPY --from=builder /code/bundle/manifests /manifests/' bundle.Dockerfile
grep -qFx 'COPY --from=builder /code/bundle/metadata /metadata/' bundle.Dockerfile
grep -qFx 'COPY --from=builder /code/bundle/tests/scorecard /tests/scorecard/' bundle.Dockerfile
grep -qFx 'LABEL version="1.4.0"' bundle.Dockerfile
grep -qFx 'LABEL operators.operatorframework.io.bundle.channels.v1=stable' bundle.Dockerfile
grep -q 'stable' bundle/metadata/annotations.yaml

# Direct invocation remains idempotent, and label updates do not alter builder values.
cp bundle.Dockerfile first.Dockerfile
VERSION=1.4.0 ./patch-bundle-dockerfile.sh
cmp first.Dockerfile bundle.Dockerfile
VERSION=1.5.0 ./patch-bundle-dockerfile.sh
grep -qFx 'LABEL version="1.5.0"' bundle.Dockerfile
sed '/^FROM scratch/,$d' bundle.Dockerfile > actual-builder
diff -u expected-builder actual-builder

# No hardcoded fallback: missing builder must fail without changing the file.
printf 'FROM scratch\nCOPY bundle/manifests /manifests/\n' > bundle.Dockerfile
cp bundle.Dockerfile no-builder.Dockerfile
if VERSION=1.4.0 ./patch-bundle-dockerfile.sh; then
    echo 'Expected failure for missing builder stage' >&2
    exit 1
fi
cmp no-builder.Dockerfile bundle.Dockerfile

# Refuse malformed generated output rather than replacing it with only a builder.
printf 'LABEL version="broken"\n' > bundle.Dockerfile
cp bundle.Dockerfile no-bundle.Dockerfile
if VERSION=1.4.0 ./patch-bundle-dockerfile.sh saved.Dockerfile; then
    echo 'Expected failure for missing bundle stage' >&2
    exit 1
fi
cmp no-bundle.Dockerfile bundle.Dockerfile

# Exercise the make recipe: reject a builder-free snapshot before any generation.
cp "$repo_dir/Makefile" .
mkdir -p hack snapshots
cp "$script_dir/patch-bundle-dockerfile.sh" hack/
printf 'FROM scratch\n# Keep this file unchanged on failure.\n' > bundle.Dockerfile
cp bundle.Dockerfile original.Dockerfile
cp -r config original-config
cp -r bundle original-bundle
if TMPDIR="$PWD/snapshots" make bundle \
    OPERATOR_SDK="$sdk" KUSTOMIZE="$kustomize" > make-bundle.log 2>&1; then
    echo 'Expected make bundle to reject a missing builder stage' >&2
    exit 1
fi
cmp original.Dockerfile bundle.Dockerfile
diff -r original-config config
diff -r original-bundle bundle
[[ -z "$(ls -A snapshots)" ]]
grep -q 'No builder stage found' make-bundle.log
