FROM registry.access.redhat.com/ubi9/go-toolset:1.26.7 as builder
ARG IMG=registry.redhat.io/multiarch-tuning/multiarch-tuning-rhel9-operator@sha256:5a82186a034fb6404a710e6c1b5e252d6bb07d2a62e43125e74cc019c054cc47
ARG ORIGINAL_IMG=registry.ci.openshift.org/origin/multiarch-tuning-operator:main
USER 0
WORKDIR /code
COPY bundle/ bundle/

# Replace the bundle image in the repository with the one specified by the IMG build argument.
RUN chmod -R g+rwX bundle/ && \
    if [ -z "${IMG}" ] || [ -z "${ORIGINAL_IMG}" ]; then \
        echo "IMG and ORIGINAL_IMG build arguments must not be empty" && exit 1; \
    fi; \
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

FROM scratch

# Core bundle labels.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=multiarch-tuning-operator
LABEL operators.operatorframework.io.bundle.channels.v1=stable
LABEL operators.operatorframework.io.bundle.channel.default.v1=stable
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.42.3
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1
LABEL operators.operatorframework.io.metrics.project_layout=go.kubebuilder.io/v4

# Labels for testing.
LABEL operators.operatorframework.io.test.mediatype.v1=scorecard+v1
LABEL operators.operatorframework.io.test.config.v1=tests/scorecard/

# Copy files to locations specified by labels.
COPY --from=builder /code/bundle/manifests /manifests/
COPY --from=builder /code/bundle/metadata /metadata/
COPY --from=builder /code/bundle/tests/scorecard /tests/scorecard/

# Labels from hack/patch-bundle-dockerfile.sh
LABEL com.redhat.component="Multiarch Tuning Operator"
LABEL distribution-scope="public"
LABEL name="multiarch-tuning/multiarch-tuning-operator-bundle"
LABEL release="1.4.1"
LABEL version="1.4.1"
LABEL cpe="cpe:/a:redhat:multiarch_tuning_operator:1.4::el9"
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
LABEL io.openshift.tags="openshift,operator,multiarch,scheduling"
