package main

import (
	"fmt"

	//rest "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	wardlecli "kubevirt.io/kubevirt/pkg/client/clientset/versioned/typed/wardle/v1alpha1"
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", "/home/dvossel/go/src/kubevirt.io/kubevirt/cluster/vagrant/.kubeconfig")
	if err != nil {
		panic(err)
	}

	client, err := wardlecli.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	restClient := client.RESTClient()

	obj, err := restClient.Get().Resource("flunders").Namespace("default").Name("my-first-flunder").SubResource("fakesubresource").Do().Get()
	//obj, err := restClient.Get().Resource("flunders").Namespace("default").Name("my-first-flunder").Do().Get()

	fmt.Printf("obj %v.. err %v\n", obj, err)
}
