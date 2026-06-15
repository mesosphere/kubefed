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

# This script automates the download of binaries used by deployment
# and testing of KubeFed.

set -o errexit
set -o nounset
set -o pipefail

# Use DEBUG=1 ./scripts/download-binaries.sh to get debug output
curl_args="-fsSL"
[[ -z "${DEBUG:-""}" ]] || {
  set -x
  curl_args="-fL"
}

logEnd() {
  local msg='done.'
  [ "$1" -eq 0 ] || msg='Error downloading assets'
  echo "$msg"
}
trap 'logEnd $?' EXIT

echo "About to download some binaries. This might take a while..."

root_dir="$(cd "$(dirname "$0")/.." ; pwd)"
dest_dir="${root_dir}/bin"
mkdir -p "${dest_dir}"

platform=$(uname -s|tr A-Z a-z)
arch=$(uname -m)
case "${arch}" in
  x86_64)  arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
esac

kb_version="2.3.1"
kb_tgz="kubebuilder_${kb_version}_${platform}_amd64.tar.gz"
kb_url="https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${kb_version}/${kb_tgz}"
curl "${curl_args}" "${kb_url}" \
  | tar xzP -C "${dest_dir}" --strip-components=2

go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
source <(setup-envtest use -p env 1.31.x)

echo "KUBEBUILDER_ASSETS is set to ${KUBEBUILDER_ASSETS}"

helm_version="3.13.0"
helm_tgz="helm-v${helm_version}-${platform}-${arch}.tar.gz"
helm_url="https://get.helm.sh/$helm_tgz"
curl "${curl_args}" "${helm_url}" \
    | tar xzP -C "${dest_dir}" --strip-components=1 "${platform}-${arch}/helm"

kubectl_version="v1.35.0"
curl -Lo "${dest_dir}/kubectl" "https://dl.k8s.io/release/${kubectl_version}/bin/${platform}/${arch}/kubectl"
(cd "${dest_dir}" && \
 echo "$(curl -L "https://dl.k8s.io/release/${kubectl_version}/bin/${platform}/${arch}/kubectl.sha256")  kubectl" | \
   shasum -a 256 --check
)
chmod +x "${dest_dir}/kubectl"

golint_version="1.64.8"
golint_dir="golangci-lint-${golint_version}-${platform}-${arch}"
golint_tgz="${golint_dir}.tar.gz"
golint_url="https://github.com/golangci/golangci-lint/releases/download/v${golint_version}/${golint_tgz}"
curl "${curl_args}" "${golint_url}" \
    | tar xzP -C "${dest_dir}" --strip-components=1 "${golint_dir}/golangci-lint"

# Install go-bindata tool
pushd ${root_dir}/tools
GOBIN=${dest_dir} go install github.com/go-bindata/go-bindata/v3/go-bindata
popd

echo    "# destination:"
echo    "#   ${dest_dir}"
echo    "# versions:"
echo -n "#   kubectl:        "; "${dest_dir}/kubectl" version --client
echo -n "#   kubebuilder:    "; "${dest_dir}/kubebuilder" version
echo -n "#   helm:           "; "${dest_dir}/helm" version --client --short
echo -n "#   golangci-lint:  "; "${dest_dir}/golangci-lint" --version
echo -n "#   go-bindata:     "; "${dest_dir}/go-bindata" -version

"${root_dir}/scripts/download-e2e-binaries.sh"
