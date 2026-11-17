#!/bin/bash

set -e

# Generate or accept version
# Priority: 1) command line arg, 2) OPERATOR_VERSION env var, 3) generate from Makefile + commit
if [ -n "$1" ]; then
    VERSION="$1"
    echo "Using version from argument: $VERSION"
elif [ -n "$OPERATOR_VERSION" ]; then
    VERSION="$OPERATOR_VERSION"
    echo "Using version from OPERATOR_VERSION environment: $VERSION"
else
    # Generate version from Makefile + commit SHA
    echo "Generating version dynamically..."

    # Read base version from Makefile
    if [ -f Makefile ]; then
        BASE_VERSION=$(grep -E "^VERSION \?=" Makefile | awk '{print $3}')
    else
        echo "❌ ERROR: Makefile not found and VERSION not provided"
        echo "Usage: $0 <version>"
        echo "  or set OPERATOR_VERSION environment variable"
        exit 1
    fi

    if [ -z "$BASE_VERSION" ]; then
        echo "❌ ERROR: Could not read VERSION from Makefile"
        exit 1
    fi

    # Get commit SHA from environment variable (set by pipeline) or git
    COMMIT_SHA_VALUE="${COMMIT_SHA:-}"
    if [ -z "$COMMIT_SHA_VALUE" ] && command -v git &> /dev/null && [ -d .git ]; then
        COMMIT_SHA_VALUE=$(git rev-parse HEAD 2>/dev/null || echo "")
    fi

    # If we have a commit SHA, append it to the version
    if [ -n "$COMMIT_SHA_VALUE" ]; then
        COMMIT_SHORT="${COMMIT_SHA_VALUE:0:7}"
        VERSION="${BASE_VERSION}+${COMMIT_SHORT}"
        echo "Generated version: $VERSION (from Makefile: $BASE_VERSION + commit: $COMMIT_SHORT)"
    else
        VERSION="$BASE_VERSION"
        echo "Using base version from Makefile: $VERSION (no commit SHA available)"
    fi
fi

echo "Bumping version to: $VERSION"

# Extract major.minor version for CPE label (e.g., 1.3.4 -> 1.3, 1.3.0+abc1234 -> 1.3)
MAJOR_MINOR=$(echo "$VERSION" | sed -E 's/^([0-9]+\.[0-9]+).*/\1/')
echo "CPE version (major.minor): $MAJOR_MINOR"

# Escape version for use in sed (handles dots and dashes)
VERSION_ESCAPED=$(echo "$VERSION" | sed 's/[.]/\\./g')

echo "Updating version references..."

# Update config/manifests/bases/multiarch-tuning-operator.clusterserviceversion.yaml
sed -i "s/^  version: .*/  version: ${VERSION}/" config/manifests/bases/multiarch-tuning-operator.clusterserviceversion.yaml
sed -i "s/^  name: multiarch-tuning-operator\.v.*/  name: multiarch-tuning-operator.v${VERSION}/" config/manifests/bases/multiarch-tuning-operator.clusterserviceversion.yaml

# Update deploy/base/operators.coreos.com/subscriptions/openshift-multiarch-tuning-operator/subscription.yaml
sed -i "s/^  startingCSV: multiarch-tuning-operator\.v.*/  startingCSV: multiarch-tuning-operator.v${VERSION}/" deploy/base/operators.coreos.com/subscriptions/openshift-multiarch-tuning-operator/subscription.yaml

# Update index.base.yaml (channel entry name)
sed -i "s/^    name: multiarch-tuning-operator\.v.*/    name: multiarch-tuning-operator.v${VERSION}/" index.base.yaml

# Update Dockerfiles
sed -i "s/^LABEL release=.*/LABEL release=\"${VERSION}\"/" Dockerfile
sed -i "s/^LABEL version=.*/LABEL version=\"${VERSION}\"/" Dockerfile
sed -i "s/^LABEL cpe=.*/LABEL cpe=\"cpe:\/a:redhat:multiarch_tuning_operator:${MAJOR_MINOR}::el9\"/" Dockerfile

sed -i "s/^LABEL release=.*/LABEL release=\"${VERSION}\"/" bundle.Dockerfile
sed -i "s/^LABEL version=.*/LABEL version=\"${VERSION}\"/" bundle.Dockerfile
sed -i "s/^LABEL cpe=.*/LABEL cpe=\"cpe:\/a:redhat:multiarch_tuning_operator:${MAJOR_MINOR}::el9\"/" bundle.Dockerfile

# Update Makefile
sed -i "s/^VERSION ?= .*/VERSION ?= ${VERSION}/" Makefile

# Update bundle files directly (instead of running make bundle)
echo "Updating bundle files..."

# Update bundle/manifests/multiarch-tuning-operator.clusterserviceversion.yaml
if [ -f bundle/manifests/multiarch-tuning-operator.clusterserviceversion.yaml ]; then
    sed -i "s/^  version: .*/  version: ${VERSION}/" bundle/manifests/multiarch-tuning-operator.clusterserviceversion.yaml
    sed -i "s/^  name: multiarch-tuning-operator\.v.*/  name: multiarch-tuning-operator.v${VERSION}/" bundle/manifests/multiarch-tuning-operator.clusterserviceversion.yaml
else
    echo "⚠️  Warning: bundle/manifests/multiarch-tuning-operator.clusterserviceversion.yaml not found, skipping"
fi

# Update bundle/metadata/annotations.yaml
if [ -f bundle/metadata/annotations.yaml ]; then
    # The annotations.yaml has version in several places, update all
    sed -i "s/operators\.operatorframework\.io\.bundle\.channels\.v1: .*/operators.operatorframework.io.bundle.channels.v1: stable/" bundle/metadata/annotations.yaml
else
    echo "⚠️  Warning: bundle/metadata/annotations.yaml not found, skipping"
fi

echo "✅ Version bumped to: $VERSION"
echo "✅ All version references updated"