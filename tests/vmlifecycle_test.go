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

package tests_test

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"

	"kubevirt.io/kubevirt/pkg/api/v1"
	"kubevirt.io/kubevirt/pkg/kubecli"
	"kubevirt.io/kubevirt/tests"
)

var _ = Describe("Vmlifecycle", func() {

	primaryNodeName := os.Getenv("primary_node_name")
	if primaryNodeName == "" {
		primaryNodeName = "master"
	}
	dockerTag := os.Getenv("docker_tag")
	if dockerTag == "" {
		dockerTag = "latest"
	}

	flag.Parse()

	virtClient, err := kubecli.GetKubevirtClient()
	tests.PanicOnError(err)

	var vm *v1.VirtualMachine

	BeforeEach(func() {
		tests.BeforeTestCleanup()
		vm = tests.NewRandomVM()
	})

	Context("New VM given", func() {

		It("Should be accepted on POST", func() {
			err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Error()
			Expect(err).To(BeNil())
		})

		It("Should reject posting the same VM a second time", func() {
			err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Error()
			Expect(err).To(BeNil())
			b, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).DoRaw()
			Expect(err).ToNot(BeNil())
			status := metav1.Status{}
			err = json.Unmarshal(b, &status)
			Expect(err).To(BeNil())
			Expect(status.Code).To(Equal(int32(http.StatusConflict)))
		})

		It("Should return 404 if VM does not exist", func() {
			b, err := virtClient.RestClient().Get().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Name("nonexistnt").DoRaw()
			Expect(err).ToNot(BeNil())
			status := metav1.Status{}
			err = json.Unmarshal(b, &status)
			Expect(err).To(BeNil())
			Expect(status.Code).To(Equal(int32(http.StatusNotFound)))
		})

		It("Should start the VM on POST", func(done Done) {
			obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Get()
			Expect(err).To(BeNil())
			tests.WaitForSuccessfulVMStart(obj)

			close(done)
		}, 30)

		It("Virt-launcher should attach to a started VM", func(done Done) {
			obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Get()
			Expect(err).To(BeNil())
			tests.WaitForSuccessfulVMStart(obj)

			namespace := vm.GetObjectMeta().GetNamespace()
			domain := vm.GetObjectMeta().GetName()
			label := fmt.Sprintf("kubevirt.io/domain in (%s)", domain)

			tests.ContainerHasLogString(virtClient, "compute", namespace, label, int64(0), int64(100), "Found PID for qemu")

			close(done)
		}, 30)

		Context("New VM which can't be started", func() {

			It("Should retry starting the VM", func(done Done) {
				vm.Spec.Domain.Devices.Interfaces[0].Source.Network = "nonexistent"
				obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Get()
				Expect(err).To(BeNil())

				retryCount := 0
				tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().Watch(func(event *k8sv1.Event) bool {
					if event.Type == "Warning" && event.Reason == v1.SyncFailed.String() {
						retryCount++
						if retryCount >= 2 {
							// Done, two retries is enough
							return true
						}
					}
					return false
				})
				close(done)
			}, 30)

			It("Should stop retrying invalid VM and go on to latest change request", func(done Done) {
				vm.Spec.Domain.Devices.Interfaces[0].Source.Network = "nonexistent"
				obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Get()
				Expect(err).To(BeNil())

				// Wait until we see that starting the VM is failing
				event := tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().WaitFor(tests.WarningEvent, v1.SyncFailed)
				Expect(event.Message).To(ContainSubstring("nonexistent"))

				_, err = virtClient.RestClient().Delete().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Name(vm.GetObjectMeta().GetName()).Do().Get()
				Expect(err).To(BeNil())

				// Check that the definition is deleted from the host
				tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().WaitFor(tests.NormalEvent, v1.Deleted)

				close(done)

			}, 30)
		})

		Context("New VM that will be killed", func() {
			It("Should be in Failed phase", func(done Done) {
				obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Get()
				Expect(err).To(BeNil())

				nodeName := tests.WaitForSuccessfulVMStart(obj)
				_, ok := obj.(*v1.VirtualMachine)
				Expect(ok).To(BeTrue(), "Object is not of type *v1.VM")
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(10 * time.Second)
				err = pkillAllVms(virtClient, nodeName, dockerTag)
				Expect(err).To(BeNil())

				tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().WaitFor(tests.WarningEvent, v1.Stopped)

				Expect(func() v1.VMPhase {
					vm := &v1.VirtualMachine{}
					err := virtClient.RestClient().Get().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Name(obj.(*v1.VirtualMachine).ObjectMeta.Name).Do().Into(vm)
					Expect(err).ToNot(HaveOccurred())
					return vm.Status.Phase
				}()).To(Equal(v1.Failed))

				close(done)
			}, 50)
			It("should be left alone by virt-handler", func(done Done) {
				obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Get()
				Expect(err).To(BeNil())

				nodeName := tests.WaitForSuccessfulVMStart(obj)
				_, ok := obj.(*v1.VirtualMachine)
				Expect(ok).To(BeTrue(), "Object is not of type *v1.VM")
				Expect(err).ToNot(HaveOccurred())

				err = pkillAllVms(virtClient, nodeName, dockerTag)
				Expect(err).To(BeNil())

				// Wait for stop event of the VM
				tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().WaitFor(tests.WarningEvent, v1.Stopped)

				// Wait for some time and see if a sync event happens on the stopped VM
				event := tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().Timeout(5*time.Second).
					SinceWatchedObjectResourceVersion().WaitFor(tests.WarningEvent, v1.SyncFailed)
				Expect(event).To(BeNil(), "virt-handler tried to sync on a VM in final state")

				close(done)
			}, 50)
		})

		Context("New VM when virt-launcher crashes", func() {
			It("should stop and be in Failed phase", func(done Done) {
				obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Body(vm).Do().Get()
				Expect(err).To(BeNil())

				nodeName := tests.WaitForSuccessfulVMStart(obj)
				_, ok := obj.(*v1.VirtualMachine)
				Expect(ok).To(BeTrue(), "Object is not of type *v1.VM")
				Expect(err).ToNot(HaveOccurred())

				time.Sleep(10 * time.Second)
				err = pkillAllLaunchers(virtClient, nodeName, dockerTag)
				Expect(err).To(BeNil())

				tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().WaitFor(tests.WarningEvent, v1.Stopped)

				Expect(func() v1.VMPhase {
					vm := &v1.VirtualMachine{}
					err := virtClient.RestClient().Get().Resource("virtualmachines").Namespace(tests.NamespaceTestDefault).Name(obj.(*v1.VirtualMachine).ObjectMeta.Name).Do().Into(vm)
					Expect(err).ToNot(HaveOccurred())
					return vm.Status.Phase
				}()).To(Equal(v1.Failed))

				close(done)
			}, 50)
		})

		Context("in a non-default namespace", func() {
			table.DescribeTable("Should log libvirt start and stop lifecycle events of the domain", func(namespace string) {

				_, exists := os.LookupEnv("JENKINS_HOME")
				if exists {
					Skip("Skip log query tests for JENKINs ci test environment")
				}
				vm = tests.NewRandomVMWithNS(namespace)
				virtHandlerPod, err := kubecli.NewVirtHandlerClient(virtClient).ForNode(primaryNodeName).Pod()
				Expect(err).ToNot(HaveOccurred())

				// Make sure we schedule the VM to master
				vm.Spec.NodeSelector = map[string]string{"kubernetes.io/hostname": primaryNodeName}

				// Start the VM and wait for the confirmation of the start
				obj, err := virtClient.RestClient().Post().Resource("virtualmachines").Namespace(vm.GetObjectMeta().GetNamespace()).Body(vm).Do().Get()
				Expect(err).ToNot(HaveOccurred())
				tests.WaitForSuccessfulVMStart(obj)

				// verify virt-handler log has "Domain is in state Running" log message
				tests.PodHasLogRegex(virtClient, virtHandlerPod, "virt-handler", int64(120), fmt.Sprintf("(name=%s)[^\n]+(kind=Domain)[^\n]+(Domain is in state Running)", vm.GetObjectMeta().GetName()))

				// Check the VM Namespace
				Expect(vm.GetObjectMeta().GetNamespace()).To(Equal(namespace))

				// Delete the VM and wait for the confirmation of the delete
				_, err = virtClient.RestClient().Delete().Resource("virtualmachines").Namespace(vm.GetObjectMeta().GetNamespace()).Name(vm.GetObjectMeta().GetName()).Do().Get()
				Expect(err).To(BeNil())
				tests.NewObjectEventWatcher(obj).SinceWatchedObjectResourceVersion().WaitFor(tests.NormalEvent, v1.Deleted)

				// verify virt-handler log has "Domain deleted" log message
				tests.PodHasLogRegex(virtClient, virtHandlerPod, "virt-handler", int64(120), fmt.Sprintf("(name=%s)[^\n]+(kind=Domain)[^\n]+(Domain deleted)", vm.GetObjectMeta().GetName()))

			},
				table.Entry(tests.NamespaceTestDefault, tests.NamespaceTestDefault),
				table.Entry(tests.NamespaceTestAlternative, tests.NamespaceTestAlternative),
			)
		})
	})
})

func renderPkillAllJob(dockerTag string, processName string) *k8sv1.Pod {
	job := k8sv1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "vm-killer",
			Labels: map[string]string{
				v1.AppLabel: "test",
			},
		},
		Spec: k8sv1.PodSpec{
			RestartPolicy: k8sv1.RestartPolicyNever,
			Containers: []k8sv1.Container{
				{
					Name:  "vm-killer",
					Image: "kubevirt/vm-killer:" + dockerTag,
					Command: []string{
						"pkill",
						"-9",
						processName,
					},
					SecurityContext: &k8sv1.SecurityContext{
						Privileged: newBool(true),
						RunAsUser:  new(int64),
					},
				},
			},
			HostPID: true,
			SecurityContext: &k8sv1.PodSecurityContext{
				RunAsUser: new(int64),
			},
		},
	}

	return &job
}

func pkillAllLaunchers(virtCli kubecli.KubevirtClient, node, dockerTag string) error {
	job := renderPkillAllJob(dockerTag, "virt-launcher")
	job.Spec.NodeName = node
	_, err := virtCli.CoreV1().Pods(tests.NamespaceTestDefault).Create(job)

	return err
}

func pkillAllVms(virtCli kubecli.KubevirtClient, node, dockerTag string) error {
	job := renderPkillAllJob(dockerTag, "qemu")
	job.Spec.NodeName = node
	_, err := virtCli.CoreV1().Pods(tests.NamespaceTestDefault).Create(job)

	return err
}

func newBool(x bool) *bool {
	return &x
}
