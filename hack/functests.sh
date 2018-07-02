#!/bin/bash
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

source hack/common.sh
source hack/config.sh

functest_docker_prefix=${manifest_docker_prefix-${docker_prefix}}

if [[ ${TARGET} == openshift* ]]; then
    oc=${kubectl}
fi

# Include CDI in dev providers for now.
if [[ "$TARGET" =~ .*-dev ]]; then
    _kubectl create -f ${MANIFESTS_OUT_DIR}/optional/cdi-controller-deployment.yaml -R $i
fi

${TESTS_OUT_DIR}/tests.test -kubeconfig=${kubeconfig} -tag=${docker_tag} -prefix=${functest_docker_prefix} -oc-path=${oc} -kubectl-path=${kubectl} -has-cdi -test.timeout 90m ${FUNC_TEST_ARGS}
