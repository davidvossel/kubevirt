/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2017 Red Hat, Inc.
 *
 */

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"strings"

	"github.com/emicklei/go-restful-openapi"
	"github.com/go-openapi/spec"
	"github.com/spf13/pflag"

	"k8s.io/kube-openapi/pkg/common"

	"kubevirt.io/kubevirt/pkg/api/v1"
	klog "kubevirt.io/kubevirt/pkg/log"
	"kubevirt.io/kubevirt/pkg/virt-api"
)

func dumpOpenApiSpec(dumppath *string) {
	openapispec := restfulspec.BuildSwagger(virt_api.CreateOpenAPIConfig())
	data, err := json.MarshalIndent(openapispec, " ", " ")
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile(*dumppath, data, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func dumpOpenApiSpecV3(dumppath *string) {

	getDefinitionName := func(name string) (string, spec.Extensions) {
		return name[strings.LastIndex(name, "/")+1:], nil
	}

	getRef := func(name string) spec.Ref {
		defName, _ := getDefinitionName(name)
		return spec.MustCreateRef("#/definitions/" + common.EscapeJsonPointer(defName))
	}

	definitions := v1.GetOpenAPIDefinitions(getRef)

	// TODO /vendor/k8s.io/kube-openapi/pkg/builder/openapi.go
	// look at buildDefinitionRecursively.
	// Basically you want to give a name, like "v1.VirtualMachine" and for it
	// to build all the definitions specific to that object.
	// https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/#validation
	// https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.0.md

	data, err := json.MarshalIndent(definitions, " ", " ")
	if err != nil {
		log.Fatal(err)
	}
	err = ioutil.WriteFile(*dumppath, data, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	dumpapispecpath := flag.String("dump-api-spec-path", "openapi.json", "Path to OpenApi dump.")
	dumpapispecv3path := flag.String("dump-api-spec-v3-path", "openapi-v3.json", "Path to OpenApiV3 dump.")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	// client-go requires a config or a master to be set in order to configure a client
	pflag.Set("master", "http://127.0.0.1:4321")
	pflag.Parse()

	klog.InitializeLogging("openapispec")

	// arguments for NewVirtAPIApp have no influence on the generated spec
	app := virt_api.VirtAPIApp{}
	app.Compose()
	dumpOpenApiSpec(dumpapispecpath)
	dumpOpenApiSpecV3(dumpapispecv3path)
}
