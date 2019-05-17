#!/usr/bin/env bash
#
# This file is part of the KubeVirt project
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
#
# Copyright 2017 Red Hat, Inc.
#

set -e

DOCKER_TAG=${DOCKER_TAG:-devel}
DOCKER_TAG_ALT=${DOCKER_TAG_ALT:-devel_alt}

source hack/common.sh
source hack/config.sh

_auto_detected_latest_release_tag=$(curl -s https://github.com/kubevirt/kubevirt/releases/latest | grep -o "v[0-9]\.[0-9]*\.[0-9]*")

latest_release_tag=${LATEST_RELEASE_TAG:-$_auto_detected_latest_release_tag}
latest_release_registry=${LATEST_RELEASE_REGISTRY:-"index.docker.io/kubevirt"}

functest_docker_prefix=${manifest_docker_prefix-${docker_prefix}}

if [[ ${TARGET} == openshift* ]]; then
    oc=${kubectl}
fi

${TESTS_OUT_DIR}/tests.test -kubeconfig=${kubeconfig} -container-tag=${docker_tag} -container-tag-alt=${docker_tag_alt} -container-prefix=${functest_docker_prefix} -oc-path=${oc} -kubectl-path=${kubectl} -test.timeout 210m ${FUNC_TEST_ARGS} -installed-namespace=${namespace} -latest-release-tag=${latest_release_tag} -latest-release-registry=${latest_release_registry}
