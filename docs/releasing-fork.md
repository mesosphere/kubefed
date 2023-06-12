## Releasing kubefed from `mesosphere/kubefed`

This repo is primarily used for:

1. Publishing docker images on `ghcr.io` for consumption by DKP
2. Publishing helm charts on `master` branch for consumption by DKP
3. Consumption of kubefed as go module dependency in some DKP components.


In order to publish docker images, [test-and-push](../.github/workflows/test-and-push.yml) workflow is leveraged. This gets triggered upon pushing a tag. Helm charts can then be pushed by running `./scripts/build-release-artifacts.sh <RELEASE_TAG>` which should update the [charts/index.yaml](../charts/index.yaml). These changes should be merged after the above workflow has completed which might take a while.

Here is the gist of the manual release process:

1. Push a tag from `master`.
2. Wait for `test-and-push` workflow to complete.
3. Run `./scripts/build-release-artifacts.sh <RELEASE_TAG>` locally.
4. Create a PR by adding changes for [charts/index.yaml](../charts/index.yaml). Sample PR https://github.com/mesosphere/kubefed/pull/15
5. Merge the PR and the helm chart, docker image, go modules should be ready for consumption :rocket:.

Right now, we only needed to release patch versions so far. Release branches are not a thing yet as we hope to archive this repo in near future and no major/minor changes are planned.
