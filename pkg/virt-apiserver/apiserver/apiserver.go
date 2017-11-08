/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package apiserver

import (
	"fmt"
	"log"
	"net/http"

	"github.com/emicklei/go-restful"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/apimachinery/announced"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/version"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/sample-apiserver/pkg/apis/wardle/install"

	mime "kubevirt.io/kubevirt/pkg/rest"

	"kubevirt.io/kubevirt/pkg/api/v1"
	"kubevirt.io/kubevirt/pkg/virt-api/rest"
)

var (
	groupFactoryRegistry = make(announced.APIGroupFactoryRegistry)
	registry             = registered.NewOrDie("")
	Scheme               = runtime.NewScheme()
	Codecs               = serializer.NewCodecFactory(Scheme)
)

func init() {
	install.Install(groupFactoryRegistry, registry, Scheme)

	// we need to add the options to empty v1
	// TODO fix the server code to avoid this
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})

	// TODO: keep the generic API server from wanting this
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

type ExtraConfig struct {
	// Place you custom config here.
}

type Config struct {
	GenericConfig *genericapiserver.RecommendedConfig
	ExtraConfig   ExtraConfig
}

// KubeVirtServer contains state for a Kubernetes cluster master/api server.
type KubeVirtServer struct {
	GenericAPIServer *genericapiserver.GenericAPIServer
}

type completedConfig struct {
	GenericConfig genericapiserver.CompletedConfig
	ExtraConfig   *ExtraConfig
}

type CompletedConfig struct {
	// Embed a private pointer that cannot be instantiated outside of this package.
	*completedConfig
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (cfg *Config) Complete() CompletedConfig {
	c := completedConfig{
		cfg.GenericConfig.Complete(),
		&cfg.ExtraConfig,
	}

	c.GenericConfig.Version = &version.Info{
		Major: "1",
		Minor: "0",
	}

	return CompletedConfig{&c}
}

func addDeleteParams(builder *restful.RouteBuilder, ws *restful.WebService) *restful.RouteBuilder {
	return builder.Param(rest.NamespaceParam(ws)).Param(rest.NameParam(ws))
}
func fakeSubresourceEndpoint(request *restful.Request, response *restful.Response) {

	fmt.Println("HIT")
	response.WriteHeader(http.StatusOK)
}

// New returns a new instance of KubeVirtServer from the given config.
func (c completedConfig) New() (*KubeVirtServer, error) {
	genericServer, err := c.GenericConfig.New("sample-apiserver", genericapiserver.EmptyDelegate)
	if err != nil {
		return nil, err
	}

	s := &KubeVirtServer{
		GenericAPIServer: genericServer,
	}

	// TODO HERE
	// Try to add a endpoint to the go restful container object

	ctx := context.Background()

	ws, err := rest.GroupVersionProxyBase(ctx, v1.TestGroupVersion)
	if err != nil {
		log.Fatal(err)
	}

	testGVR := schema.GroupVersionResource{Group: v1.TestGroupVersion.Group, Version: v1.TestGroupVersion.Version, Resource: "test"}

	ws.Route(addDeleteParams(
		ws.DELETE(rest.ResourcePath(testGVR)).
			Produces(mime.MIME_JSON, mime.MIME_YAML).
			Consumes(mime.MIME_JSON, mime.MIME_YAML).
			Operation("deleteNamespaced"+v1.TestGroupVersionKind.Kind).
			To(fakeSubresourceEndpoint).Writes(&v1.Test{}).
			Doc("Delete a "+v1.TestGroupVersionKind.Kind+" object.").
			Returns(http.StatusOK, "Deleted", &v1.Test{}).
			Returns(http.StatusNotFound, "Not Found", nil), ws,
	))

	ws.Route(ws.GET(rest.ResourcePath(testGVR) + rest.SubResourcePath("console")).
		To(fakeSubresourceEndpoint).
		Param(restful.QueryParameter("console", "Name of the serial console to connect to")).
		Param(rest.NamespaceParam(ws)).Param(rest.NameParam(ws)).
		Operation("console").
		Doc("Open a websocket connection to a serial console on the specified VM."))

	genericServer.Handler.GoRestfulContainer.Add(ws)

	//apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(wardle.GroupName, registry, Scheme, metav1.ParameterCodec, Codecs)
	//apiGroupInfo.GroupMeta.GroupVersion = v1alpha1.SchemeGroupVersion
	//v1alpha1storage := map[string]rest.Storage{}
	//v1alpha1storage["flunders"] = wardleregistry.RESTInPeace(flunderstorage.NewREST(Scheme, c.GenericConfig.RESTOptionsGetter))
	//v1alpha1storage["fischers"] = wardleregistry.RESTInPeace(fischerstorage.NewREST(Scheme, c.GenericConfig.RESTOptionsGetter))
	//apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1storage

	//if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
	//	return nil, err
	//}

	return s, nil
}
