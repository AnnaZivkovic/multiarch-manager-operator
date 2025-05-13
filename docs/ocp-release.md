
### Branching and creation of a new development stream
This is done at the beginning of a release cycle.

1. Create new branch off of main. For example v1.0, v0.9.
2. Define a new Development Stream for the operator in the [Konflux
   cluster](https://gitlab.cee.redhat.com/releng/konflux-release-data). <br><br>
   Create a new directory
   `tenants-config/cluster/stone-prd-rh01/tenants/multiarch-tuning-ope-tenant/_base/projectl.konflux.dev/projectdevelopmentstreams/multiarch-tuning-operator-<konfluxVersion>`.
   For instance, when the branch version is `v1.0`, replace the period (`.`) with a dash (`-`). Thus, `<konfluxVersion>` would
   be formatted as `v1-0`.
   Add two files to the directory `projectdevelopmentstream.yaml` and `kustomization.yaml`.

   ```yaml
   # multiarch-tuning-operator-<konfluxVersion>/projectdevelopmentstream.yaml
   ---
   apiVersion: projctl.konflux.dev/v1beta1
   kind: ProjectDevelopmentStream
   metadata:
     name: multiarch-tuning-operator-<konfluxVersion>
   spec:

     project: multiarch-tuning-operator
     template:
       name: multiarch-tuning-operator
       values:
         - name: version
           # The branch name, formatted without the 'v' prefix (e.g., '1.0' for branch 'v1.0')
           value: <versionName>
   ```

   ```yaml
   # multiarch-tuning-operator-<konfluxVersion>/kustomization.yaml
   ---
   kind: Kustomization
   apiVersion: kustomize.config.k8s.io/v1beta1
   resources:
     - ./projectdevelopmentstream.yaml
   ```

   Update `tenants-config/cluster/stone-prd-rh01/tenants/multiarch-tuning-ope-tenant/_base/kustomization.yaml` file to
   include the newly added `projectdevelopmentstreams`.<br><br>
   Run `build-manifests.sh` to generate auxiliary files. After making the changes, create a new pull request.

3. Update the tekton files in the newly created branch. An example can be found [here](https://github.com/openshift/multiarch-tuning-operator/commit/b31311f3e642b2ba4ac71b32a675395962d5dd38).

4. Duplicate the prow config to target the new branch. Add
   `ci-operator/config/openshift/multiarch-tuning-operator/openshift-multiarch-tuning-operator-<branchVersion>.yaml` by
   copping `openshift-multiarch-tuning-operator-main.yaml` and updating references from `main` to the new branch name.
   (ex. https://github.com/openshift/release/pull/56835)
5. Create a new `ReleasePlanAdmission` file in the directory
      `konflux-release-data/config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/multiarch-tuning-ope`,
      naming it `multiarch-tuning-operator-<version>.yaml` where `<version>` is the `konfluxVersion` without the "
      v" (e.g., for `v1-0`, name it `multiarch-tuning-operator-1-0.yaml`).
      Duplicate the contents of the existing ReleasePlanAdmission from the `multiarch-tuning-operator.yaml` file, update
      the
    - tags
    - set the appropriate release notes version
    - change metadata name from `multiarch-tuning-operator` to `multiarch-tuning-operator-<version>`
    - change the spec application name to `multiarch-tuning-operator-<version>`
    - change the components to include `<version>`<br><br>
      Run `build-manifests.sh` to generate auxiliary files. After making the changes, create a new pull request.

### Release the operator (for OCP)
1. When planning to release a new version (e.g. v1.0) or a patch version (e.g., v1.0.z), select a snapshot from the
corresponding Konflux application
and ensure all post-submit checks have successfully passed.

    ```shell
    oc get snapshots --sort-by .metadata.creationTimestamp -l pac.test.appstudio.openshift.io/event-type=push,appstudio.openshift.io/application=multiarch-tuning-operator-<version>
    ```

3. Look at the results of the tests for the commit reported in the snapshot:

   ```yaml
   [ ... ]
   spec:
     application: multiarch-tuning-operator
     artifacts: { }
     components:
       - containerImage: quay.io/redhat-user-workloads/multiarch-tuning-ope-tenant/multiarch-tuning-operator-<version>/multiarch-tuning-operator-<version>@sha256:250498c137c91f8f932317d48ecacb0d336e705828d3a4163e684933b610547f
         name: multiarch-tuning-operator-<version>
         source:
           git:
             revision: d73959c925629f29f9071fd6e7d58a0f58a54399
             url: https://github.com/openshift/multiarch-tuning-operator
       - containerImage: quay.io/redhat-user-workloads/multiarch-tuning-ope-tenant/multiarch-tuning-operator-<version>/multiarch-tuning-operator-bundle-<version>@sha256:be0945723a0a5ad881d135b3f83a65b0e8fc69c0da337339587aebed4bee89a1
         name: multiarch-tuning-operator-bundle-<version>
         source:
           git:
             context: ./
             dockerfileUrl: bundle.Dockerfile
             revision: d73959c925629f29f9071fd6e7d58a0f58a54399
             url: https://github.com/openshift/multiarch-tuning-operator
     [ ... ]
   ```

4. Ensure that the `containerImage` of the operator matches the one referenced by the bundle in the selected snapshot.
   This value should not be updated manually—Konflux will automatically file the PR. If it doesn’t, there may be an
   issue with the nudging process, and you should review the `Renovate` PipelineRun logs. Once resolved, wait for the new snapshot to be made
   and restart at step 2. <br><br>
   Pull and save the pulled image to a tar file. Extract the tar file into the newly created directory and remember to extract the nested tar files.

    ```shell
    find /tmp/operator_bundle -name "*.clusterserviceversion.yaml" -print
    yq '.spec.install.spec.deployments[0].spec.template.metadata.annotations."multiarch.openshift.io/image"' <file-from-above-command>
    ```
9. Get the new snapshot triggered by the build of the merge commit at the previous point and check the build pipeline for errors and warnings.

    ```shell
    oc get snapshots --sort-by .metadata.creationTimestamp -l pac.test.appstudio.openshift.io/event-type=push,appstudio.openshift.io/application=fbc-v4-x
    ```

10. Create a new Release for the operator snapshot in the previous step. 
    - Update the [allowed tags](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/blob/main/config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/multiarch-tuning-ope/multiarch-tuning-operator-1-0.yaml#L27-28) for the release plan admission 
    ```yaml
      ...
      defaults:
        tags:
          - "1.0.0"
          - "1.0.0-{{ timestamp }}"
          - "add new tags"
      ...
    ```
    - In [comet](https://comet.engineering.redhat.com/containers/repositories/6616582895a35187a06ba2ce) add the new tag in the content streams field
    - Create a new release for the operator 
    ```yaml
    apiVersion: appstudio.redhat.com/v1alpha1
    kind: Release
    metadata:
      generateName: release-1-0-0-
      namespace: multiarch-tuning-ope-tenant
    spec:
      releasePlan: multiarch-tuning-operator-1-0-release-as-operator
      snapshot: multiarch-tuning-operator-1-0-5lr4j
      data:
        releaseNotes:
          type: RHEA
          synopsis: Red Hat Multiarch Tuning 1.0.0
          topic: >-
            The 1.0.0 release of the Red Hat Multiarch Tuning Operator.
            For more details, see [product documentation](https://docs.openshift.com/container-platform/4.17/post_installation_configuration/configuring-multi-arch-compute-machines/multiarch-tuning-operator.html).
          description: >-
            The Red Hat Multiarch Tuning Operator can be used with OpenShift Container Platform.
            Enhancements:
              - With this release, the Multiarch Tuning Operator supports custom network scenarios and cluster-wide custom registries configurations.
              - With this release, you can identify pods based on their architecture compatibility by using the pod labels that the Operator adds to newly created pods.
              - With this release, you can monitor the behavior of the Multiarch Tuning Operator by using the metrics and alerts registered in the cluster monitoring operator.
          solution: >-
            The Multiarch Tuning Operator optimizes workload management within multi-architecture clusters and in single-architecture clusters transitioning to multi-architecture environments.
            This Operator is available in the Red Hat Operators catalog that is included with OpenShift Container Platform.
            For more details, see [product documentation](https://docs.openshift.com/container-platform/4.17/post_installation_configuration/configuring-multi-arch-compute-machines/multiarch-tuning-operator.html).
          references:
            - https://docs.openshift.com/container-platform/4.17/post_installation_configuration/configuring-multi-arch-compute-machines/multiarch-tuning-operator.html
            - https://github.com/openshift/multiarch-tuning-operator
    ```
11. Watch the `status` of the `Release` Object or look in the Konflux UI to confirm that images and bundle were published
    as expected. Note that the peipeline is run in a different namespace (rhtap-releng-tenant).

### Publish a released operator in the OCP's OperatorHub's FBCs
Note: these steps need to be applied to each FBC

1. Update the `fbc-4.x` branches to use the selected container image for both the bundle and operator.
    Example [commit](https://github.com/openshift/multiarch-tuning-operator/pull/341). The process will generate and render
    a new bundle—rather than modifying the existing reference, simply add a new bundle and introduce a new channel if needed.
    When updating the version of `registry.redhat.io/openshift4/ose-operator-registry:v4.1X`, ensure that all bundles are generated with `registry.redhat.io/openshift4/ose-operator-registry:v4.14`, except for the `4.16` bundles, which must be generated with `registry.redhat.io/openshift4/ose-operator-registry:v4.13`.

    ```shell
    podman run -it --entrypoint /bin/opm -v $(pwd):/code:Z registry.redhat.io/openshift4/ose-operator-registry:v4.1X render <konflux-bundle-image> --output=yaml
    ```
    Make a pull request with the updated `index.yaml`. The commit message should resemble this [commit](https://github.com/openshift/multiarch-tuning-operator/pull/341).

9. Get the new snapshot triggered by the build of the merge commit at the previous point and check the build pipeline for errors and warnings.

    ```shell
    oc get snapshots --sort-by .metadata.creationTimestamp -l pac.test.appstudio.openshift.io/event-type=push,appstudio.openshift.io/application=fbc-v4-x
    ```

10. Create a new Release for the fbc snapshot created after the commit in the previous step. 
    Note that we can run a staging release to test the release pipeline

    ```yaml
    # oc create -f - <<EOF
    apiVersion: appstudio.redhat.com/v1alpha1
    kind: Release
    metadata:
      generateName: manual-release-
      namespace: multiarch-tuning-ope-tenant
    spec:
      releasePlan: fbc-4-1x-release-as-fbc # fbc-v4-16-release-as-staging-fbc is available for a staging release
      snapshot: fbc-<new-fbc-snapshot>
    # EOF
    ```
11. Watch the `status` of the `Release` Object or look in the Konflux UI to confirm that images and bundle were published
as expected. Note that the peipeline is run in a different namespace (rhtap-releng-tenant).

12. Once the release pipeline has succeeded view the index images provided in the task run section.
    ```yaml
    {
        "index_image": {
            "index_image": "registry-proxy.engineering.redhat.com/rh-osbs/iib:835019",
            "index_image_resolved": "registry-proxy.engineering.redhat.com/rh-osbs/iib@sha256:d594746527f8acd02af12c48d15842f02cf61a3091aede7238d222f1d0ec92c5"
        }
    }
    ```
    Take the index image and replace the registry `registry-proxy.engineering.redhat.com/rh-osbs/iib` with  `brew.registry.redhat.io`.
    The image to announce would look like `brew.registry.redhat.io:835019`

### Create a new FBC fragment for a new OCP release

The current approach is that for every new OCP release, we will have to:
1. Create New Konflux Application and Component for the FBC  
   https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/2515/diffs?commit_id=5af448016c0f5d65dd74ddffeafa27932fa9b252
2. Create a new branch in this repo like `fbc-4.16`. E.g., `fbc-4.17` for OCP 4.17
3. Update the tekton files on the new branch
   https://github.com/openshift/multiarch-tuning-operator/pull/350/commits/cf23045ba70e4806efc3690ebd786b717075a25b
3. Create a new PR in https://gitlab.cee.redhat.com/releng/konflux-release-data/-/tree/main to add the new File Based Catalog (FBC) fragment to the ReleasePlanAdmission, based on  
   a. https://gitlab.cee.redhat.com/releng/konflux-release-data/-/blob/main/config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/multiarch-tuning-ope/multiarch-tuning-operator-fbc-prod-index.yaml  
   b. https://gitlab.cee.redhat.com/releng/konflux-release-data/-/blob/main/config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/multiarch-tuning-ope/multiarch-tuning-operator-fbc-staging-index.yaml  
   Example PR: https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/3039
4. Adjust the BASE_IMAGE argument for the `build-args` parameter in the Konflux PipelineRun.
5. Adjust the graphs as needed in the branches.

If the release is a major version or introduced significant
[architectural changes](https://docs.engineering.redhat.com/pages/viewpage.action?pageId=402429315),
ensure that the threat model is reviewed and updated accordingly.

SAST findings will be processed according to:

- https://spaces.redhat.com/display/PRODSEC/SAST+Workflow
- https://spaces.redhat.com/display/PRODSEC/Weakness+Management+Standard
- https://spaces.redhat.com/display/PRODSEC/Security+Errata+Standard

In general, SAST findings cause a PR to fail to merge.
It's the resposibility of the PR author to review the findings.

In case a finding is not a false positive, it needs to be classified as weakness or vulnerability
(if in doubt, contact the Security Architect for help).
Once classified, an appropriate Jira issue needs to be created (ask Security Architect for guidance).

The issues will have to be remediated according to the timelines set in the standard.

If malware is detected, contact prodsec@redhat.com immediately.

## Pin to a new Golang and K8S API version

1. Update go version in base images 
   - Update the Dockerfiles to use a base image with the desired Golang version (it should be the one used by k8s.io/api
   or openshift/api)
   - The function `getCorrectHostmountAnyUIDSCC` relies on the Kubernetes version to determine which SCC is used for the `podplacementconfig` pod's hostPath mounts. Please double-check the code to ensure that the Kubernetes version aligns with the corresponding OpenShift version and has not been inadvertently upgraded or downgraded.
   - Update the Makefile to use the new Golang version base image (BUILD_IMAGE variable)
   - Check if updated references are needed in .tekton for konflux 
   - Commit the changes to the Golang version
4. Update go.mod
   - Update the k8s libraries in the go.mod file to the desired version 
   - Update the dependencies with `go get -u`, and ensure no new version of the k8s API is used
    ```shell
    go mod download
    go mod tidy
    go mod verify
    ```
   - Commit the changes to go.mod and go.sum
7. Update the vendor/ folder and commit changes 

```shell
rm -rf vendor/
go mod vendor
```
4. Update the tools in the Makefile to the desired version and commit changes:

```makefile
# https://github.com/kubernetes-sigs/kustomize/releases
KUSTOMIZE_VERSION ?= v5.4.3
# https://github.com/kubernetes-sigs/controller-tools/releases
CONTROLLER_TOOLS_VERSION ?= v0.16.1
# https://github.com/kubernetes-sigs/controller-runtime/branches
SETUP_ENVTEST_VERSION ?= release-0.18
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.3
# https://github.com/golangci/golangci-lint/releases
GOLINT_VERSION = v1.60.1
```

5. Run and commit the following. Address any warnings or errors that may occur
```shell
make generate
make manifests
make bundle
```
6. Run the tests and ensure everything is building and working as expected. Look for deprecation warnings PRs in the
    controller-runtime repository.
```shell
make docker-build
make build
make test
```

7. Commit any other changes to the code, if any
8. Create a PR with the changes.
9. Update this document with any changes 

Example log:

```
02eb25dd (HEAD -> update-go) Add info in the ocp-release.md doc about k8s and golang upgrade
8e2d3389 Add info in the ocp-release.md doc about k8s and golang upgrade
46e7b338 Update code after Golang, pivot to k8s 1.30.4 and dependencies upgrade
787dfb2a Update tools in Makefile
289ebaa2 go mod vendor
cda73fe1 pin K8S API to v0.30.4 and set go minimum version to 1.22.5
e511fdce Update go version in base images to 1.22
```

Example PRs: https://github.com/openshift/multiarch-tuning-operator/pull/225, https://github.com/openshift/multiarch-tuning-operator/pull/542

The PR in the repo may need to be paired with one in the Prow config:
see https://github.com/openshift/release/pull/55728/commits/707fa080a66d8006c4a69e452a4621ed54f67cf6 as an example

# Bumping the operator version

To bump the operator version, run the following command:

```shell
make version VERSION=1.0.1
```

Also verify that no other references to the previous versions are present in the codebase.
If so, update hack/bump-version.sh to include any further patches required to update the version.