#!/usr/bin/env bash

# Copyright 2019 The Kubernetes Authors.
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

set -eou pipefail

GIT_COMMIT_SHA=${GITHUB_SHA:-$(git rev-parse HEAD)}
BASE_BRANCH=${GITHUB_BASE_REF:-}

if [[ -z "${BASE_BRANCH}" ]]; then
  if git rev-parse --verify --quiet origin/main >/dev/null; then
    BASE_BRANCH="origin/main"
  elif git rev-parse --verify --quiet main >/dev/null; then
    BASE_BRANCH="main"
  elif git rev-parse --verify --quiet origin/master >/dev/null; then
    BASE_BRANCH="origin/master"
  elif git rev-parse --verify --quiet master >/dev/null; then
    BASE_BRANCH="master"
  else
    echo "Unable to determine base branch for doc diff check"
    exit 1
  fi
else
  if git rev-parse --verify --quiet "origin/${BASE_BRANCH}" >/dev/null; then
    BASE_BRANCH="origin/${BASE_BRANCH}"
  fi
fi

CHANGED_FILES=$(git diff --name-only "${BASE_BRANCH}"..."${GIT_COMMIT_SHA}")

[[ -z $CHANGED_FILES ]] && exit 1

for CHANGED_FILE in $CHANGED_FILES; do
  if ! [[ $CHANGED_FILE =~ ^docs/ || $CHANGED_FILE =~ .md$ ]]; then
    exit 1
  fi
done
