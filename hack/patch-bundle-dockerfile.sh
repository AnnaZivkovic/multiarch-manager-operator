#!/bin/bash

set -euo pipefail

# Extract major.minor version for CPE label (e.g., 1.3.4 -> 1.3)
MAJOR_MINOR=$(echo "${VERSION:-0.9.0}" | sed -E 's/^([0-9]+\.[0-9]+).*/\1/')

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

# SDK overwrites the Dockerfile, so make bundle supplies its pre-generation snapshot.
BUILDER_SOURCE=${1:-bundle.Dockerfile}
TEMP_FILE=$(mktemp)
trap 'rm -f "$TEMP_FILE"' EXIT
awk '
    toupper($1) == "FROM" {
        if (builder) exit
        if (tolower($NF) == "builder") builder = 1
    }
    builder { print }
' "$BUILDER_SOURCE" > "$TEMP_FILE"
if [[ ! -s "$TEMP_FILE" ]]; then
    echo "No builder stage found in $BUILDER_SOURCE" >&2
    exit 1
fi

# Keep generated bundle labels and COPY instructions, not its overwritten builder.
if ! grep -q '^FROM scratch' bundle.Dockerfile; then
    echo "No bundle stage found in bundle.Dockerfile" >&2
    exit 1
fi
awk '/^FROM scratch/ { bundle = 1 } bundle { print }' bundle.Dockerfile >> "$TEMP_FILE"

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

# Command substitution trims trailing newlines so repeated patching stays idempotent.
CONTENT=$(sed '/# Labels from hack\/patch-bundle-dockerfile.sh/,$d' "$TEMP_FILE")
printf '%s\n\n%s\n' "$CONTENT" "$LABELS" > "$TEMP_FILE"

# Replace the original file
mv "$TEMP_FILE" bundle.Dockerfile
