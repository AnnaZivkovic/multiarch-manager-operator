#!/bin/bash

# Extract major.minor version for CPE label (e.g., 1.3.4 -> 1.3)
MAJOR_MINOR=$(echo "${VERSION:-0.9.0}" | sed -E 's/^([0-9]+\.[0-9]+).*/\1/')

# shellcheck disable=SC2016
# shellcheck disable=SC1004
BUILDER_STAGE='FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7 as builder
ARG IMG=registry.redhat.io/multiarch-tuning/multiarch-tuning-rhel9-operator@sha256:5a82186a034fb6404a710e6c1b5e252d6bb07d2a62e43125e74cc019c054cc47
ARG ORIGINAL_IMG=registry.ci.openshift.org/origin/multiarch-tuning-operator:main
USER 0
WORKDIR /code
COPY bundle/ bundle/

# Replace the bundle image in the repository with the one specified by the IMG build argument.
RUN chmod -R g+rwX bundle/ && \
    if grep -rq "${ORIGINAL_IMG}" bundle/; then \
        find bundle -type f -exec sed -i "s|${ORIGINAL_IMG}|${IMG}|g" {} \+ ; \
        if grep -rq "${ORIGINAL_IMG}" bundle/; then \
            echo "Failed to replace image references: original reference still present" && exit 1; \
        fi; \
        if ! grep -rq "${IMG}" bundle/; then \
            echo "Failed to replace image references: expected reference not found" && exit 1; \
        fi; \
        echo "Image references replaced"; \
    else \
        echo "Original image reference not present in bundle; skipping replacement"; \
    fi
'

LABELS='# Labels from hack/patch-bundle-dockerfile.sh
LABEL com.redhat.component="Multiarch Tuning Operator"
LABEL distribution-scope="public"
LABEL name="multiarch-tuning/multiarch-tuning-operator-bundle"
LABEL release="'"${VERSION:-0.9.0}"'"
LABEL version="'"${VERSION:-0.9.0}"'"
LABEL cpe="cpe:/a:redhat:multiarch_tuning_operator:'"${MAJOR_MINOR}"'::el9"
LABEL url="https://github.com/openshift/multiarch-tuning-operator"
LABEL vendor="Red Hat, Inc."
LABEL description="The Multiarch Tuning Operator enhances the user experience for administrators of Openshift \
                   clusters with multi-architecture compute nodes or Site Reliability Engineers willing to \
                   migrate from single-arch to multi-arch OpenShift"
LABEL io.k8s.description="The Multiarch Tuning Operator enhances the user experience for administrators of Openshift \
                   clusters with multi-architecture compute nodes or Site Reliability Engineers willing to \
                   migrate from single-arch to multi-arch OpenShift"
LABEL summary="The Multiarch Tuning Operator enhances the user experience for administrators of Openshift \
                   clusters with multi-architecture compute nodes or Site Reliability Engineers willing to \
                   migrate from single-arch to multi-arch OpenShift"
LABEL io.k8s.display-name="Multiarch Tuning Operator"
LABEL io.openshift.tags="openshift,operator,multiarch,scheduling"'

# Create temporary file with builder stage prepended
TEMP_FILE=$(mktemp)
cat <<EOF > "$TEMP_FILE"
$BUILDER_STAGE
EOF

# Append the original bundle.Dockerfile content (starting from FROM scratch)
cat bundle.Dockerfile >> "$TEMP_FILE"

# Replace the simple COPY statements with --from=builder versions
if [[ "$(uname)" == "Darwin" ]]; then
    # macOS BSD sed
    sed -i '' 's|^COPY bundle/manifests|COPY --from=builder /code/bundle/manifests|' "$TEMP_FILE"
    sed -i '' 's|^COPY bundle/metadata|COPY --from=builder /code/bundle/metadata|' "$TEMP_FILE"
    sed -i '' 's|^COPY bundle/tests/scorecard|COPY --from=builder /code/bundle/tests/scorecard|' "$TEMP_FILE"
else
    # Linux GNU sed
    sed -i 's|^COPY bundle/manifests|COPY --from=builder /code/bundle/manifests|' "$TEMP_FILE"
    sed -i 's|^COPY bundle/metadata|COPY --from=builder /code/bundle/metadata|' "$TEMP_FILE"
    sed -i 's|^COPY bundle/tests/scorecard|COPY --from=builder /code/bundle/tests/scorecard|' "$TEMP_FILE"
fi

# Remove existing labels section if present
if [[ "$(uname)" == "Darwin" ]]; then
    sed -i '' '/# Labels from hack\/patch-bundle-dockerfile.sh/,$d' "$TEMP_FILE"
else
    sed -i '/# Labels from hack\/patch-bundle-dockerfile.sh/,$d' "$TEMP_FILE"
fi

# Append labels
cat <<EOF >> "$TEMP_FILE"

$LABELS
EOF

# Replace the original file
mv "$TEMP_FILE" bundle.Dockerfile
