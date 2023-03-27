#!/usr/bin/env bash

# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script automates deployment of a KubeFed control plane to the
# current kubectl context.  It also registers the hosting cluster with
# the control plane.
#
# This script depends on kubectl and helm being installed in the path. These
# and other test binaries can be installed via the download-binaries.sh script,
# which downloads to ./bin:
#
#   $ ./scripts/download-binaries.sh
#   $ export PATH=$(pwd)/bin:${PATH}
#
# To redeploy KubeFed from scratch, prefix the deploy invocation with the deletion script:
#
#   # WARNING: The deletion script will remove KubeFed data
#   $ ./scripts/delete-kubefed.sh [join-cluster]... && ./scripts/deploy-kubefed.sh <image> [join-cluster]...
#

set -o errexit
set -o nounset
set -o pipefail

# shellcheck source=util.sh
source "${BASH_SOURCE%/*}/util.sh"

function deploy-with-helm() {
  local repository=${IMAGE_NAME%/*}
  local image_tag=${IMAGE_NAME##*/}
  local image=${image_tag%:*}
  local tag=${image_tag#*:}

  local cmd
  if [[ "${NAMESPACED}" ]]; then
    cmd="$(helm-deploy-cmd "kubefed-${NS}" "${NS}" "${repository}" "${image}" "${tag}")"
    cmd="${cmd} --set global.scope=Namespaced"
  else
    cmd="$(helm-deploy-cmd kubefed "${NS}" "${repository}" "${image}" "${tag}")"
  fi

  if [[ "${IMAGE_PULL_POLICY:-}" ]]; then
    cmd="${cmd} --set controllermanager.imagePullPolicy=${IMAGE_PULL_POLICY}"
  fi

  ${cmd}

  deployment-image-as-expected "${NS}" kubefed-admission-webhook admission-webhook "${repository}/${image}:${tag}"
  deployment-image-as-expected "${NS}" kubefed-controller-manager controller-manager "${repository}/${image}:${tag}"
}

function helm-deploy-cmd {
  # Required arguments
  local name="${1}"
  local ns="${2}"
  local repo="${3}"
  local image="${4}"
  local tag="${5}"
  if [[ "${FORCE_REDEPLOY:-}" == "y" ]]; then
    local force_redeploy_values="--set controllermanager.controller.forceRedeployment=true --set controllermanager.webhook.forceRedeployment=true"
  fi
  echo "helm upgrade -i ${name} charts/kubefed \
        --namespace ${ns} \
        --set controllermanager.controller.repository=${repo} \
        --set controllermanager.controller.image=${image} \
        --set controllermanager.controller.tag=${tag} \
        --set controllermanager.webhook.repository=${repo} \
        --set controllermanager.webhook.image=${image} \
        --set controllermanager.webhook.tag=${tag} \
        --set controllermanager.featureGates.RawResourceStatusCollection=Enabled \
        ${force_redeploy_values:-} \
        --create-namespace \
        --wait"
}

function kubefed-admission-webhook-ready() {
  local readyReplicas
  readyReplicas=$(kubectl -n "${1}" get deployments.apps kubefed-admission-webhook -o jsonpath='{.status.readyReplicas}')
  [[ "${readyReplicas}" -ge "1" ]]
}

NS="${KUBEFED_NAMESPACE:-kube-federation-system}"
IMAGE_NAME="${1:-}"
NAMESPACED="${NAMESPACED:-}"

LATEST_IMAGE_NAME=quay.io/kubernetes-multicluster/kubefed:latest
if [[ "${IMAGE_NAME}" == "${LATEST_IMAGE_NAME}" ]]; then
  USE_LATEST=y
else
  USE_LATEST=
fi

KF_NS_ARGS="--kubefed-namespace=${NS}"

if [[ -z "${IMAGE_NAME}" ]]; then
  >&2 echo "Usage: $0 <image> [join-cluster]...

<image>        should be in the form <containerregistry>/<username>/<imagename>:<tagname>

Example: ghcr.io/<username>/kubefed:test

If intending to use the docker hub as the container registry to push
the KubeFed image to, make sure to login to the local docker daemon
to ensure credentials are available for push:
  $ docker login --username <username>

<join-cluster> should be the kubeconfig context name for the additional cluster to join.
               NOTE: The host cluster is already included in the join.

"
  exit 2
fi

# Allow for no specific JOIN_CLUSTERS: they probably want to kubefedctl themselves.
shift
JOIN_CLUSTERS="${*-}"

check-command-installed kubectl
check-command-installed helm

# Build KubeFed binaries and image
if [[ "${USE_LATEST:-}" != "y" ]]; then
  cd "$(dirname "$0")/.."
  make container IMAGE_NAME="${IMAGE_NAME}"
  cd -
fi

# Use KIND_LOAD_IMAGE=y ./scripts/deploy-kubefed.sh <image> to load
# the built docker image into kind before deploying.
if [[ "${KIND_LOAD_IMAGE:-}" == "y" ]]; then
    kind load docker-image "${IMAGE_NAME}" --name="${KIND_CLUSTER_NAME:-kind}"
fi

cd "$(dirname "$0")/.."
make kubefedctl
cd -

# Deploy KubeFed resources
deploy-with-helm

# Join the host cluster
if [ -z "${NO_JOIN_HOST_CLUSTER:-}" ] ; then
    CONTEXT="$(kubectl config current-context)"
    ./bin/kubefedctl join "${CONTEXT}" --host-cluster-context "${CONTEXT}" --v=2 "${KF_NS_ARGS}" --error-on-existing=false
fi

for c in ${JOIN_CLUSTERS}; do
  ./bin/kubefedctl join "${c}" --host-cluster-context "${CONTEXT}" --v=2 "${KF_NS_ARGS}" --error-on-existing=false
done
