/*
 * This file is part of the kubevirt project
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

package cloudinit

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	kubev1 "k8s.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/api/v1"
	"kubevirt.io/kubevirt/pkg/precond"
)

const configDiskBasePath = "/var/run/libvirt/configDisk"
const configDiskFile = "configDisk.iso"

const (
	cloudInitConfigDisk = "configDisk"
)

// This is called by virt-controller before a VM is placed on a node.
// We wait for the cloud-init data to be avaiable for the VM to consume
// before letting the VM be placed.
func IsAvailable(vm *v1.VM, pod *kubev1.Pod) bool {
	if vm.Spec.CloudInit == nil {
		return true
	}

	switch vm.Spec.CloudInit.Type {
	case cloudInitConfigDisk:
		// Wait for config-disk container to become ready
		for _, containerStatus := range pod.Status.ContainerStatuses {
			if strings.Contains(containerStatus.Name, "config-disk") == false {
				// only check fpr config-disk containers
				continue
			}

			if containerStatus.Ready == false {
				return false
			}
		}
	}
	return true
}

// This is called right before a VM is defined with libvirt.
// If the cloud-init type requires altering the domain, this
// is the place to do that.
func InjectDomainData(vm *v1.VM) (*v1.VM, error) {
	if vm.Spec.CloudInit == nil {
		return vm, nil
	}

	err := ValidateArgs(vm)
	if err != nil {
		return vm, err
	}

	switch vm.Spec.CloudInit.Type {
	case cloudInitConfigDisk:
		filePath := fmt.Sprintf("%s/%s", getConfigDiskPath(vm), configDiskFile)

		newDisk := v1.Disk{}
		newDisk.Type = "file"
		newDisk.Device = "disk"
		newDisk.Driver = &v1.DiskDriver{
			Type: "raw",
			Name: "qemu",
		}
		newDisk.Source.File = filePath
		newDisk.Target = v1.DiskTarget{
			Device: vm.Spec.CloudInit.ConfigDiskTarget,
			Bus:    "virtio",
		}

		vm.Spec.Domain.Devices.Disks = append(vm.Spec.Domain.Devices.Disks, newDisk)
	default:
		return vm, errors.New(fmt.Sprintf("Unknown CloudInit type %s", vm.Spec.CloudInit.Type))
	}

	return vm, nil
}

// This function removes any local data associated with cloud-init
// Not all cloud-init types require local data.
func RemoveLocalData(vm *v1.VM) {
	if vm.Spec.CloudInit == nil {
		return
	}

	switch vm.Spec.CloudInit.Type {
	case cloudInitConfigDisk:
		os.RemoveAll(getConfigDiskPath(vm))
	}
}

// This function removes any distributed cloud-init data that might
// be hosted by something like a meta-data server.
func RemoveClusterData(vm *v1.VM) {
	if vm.Spec.CloudInit == nil {
		return
	}

	// This function exists as an entry point for future cloud-init
	// types that require distributed data.
	switch vm.Spec.CloudInit.Type {
	case cloudInitConfigDisk:
		// configDisk does not require cluster cleanup
		break
	}
}

func getConfigDiskPath(vm *v1.VM) string {
	namespace := precond.MustNotBeEmpty(vm.GetObjectMeta().GetNamespace())
	domain := precond.MustNotBeEmpty(vm.GetObjectMeta().GetName())

	return fmt.Sprintf("%s/%s/%s", configDiskBasePath, namespace, domain)
}

func ValidateArgs(vm *v1.VM) error {
	if vm.Spec.CloudInit == nil {
		return nil
	}

	switch vm.Spec.CloudInit.Type {
	case cloudInitConfigDisk:
		if vm.Spec.CloudInit.UserDataBase64 == "" {
			return errors.New(fmt.Sprintf("userDataBase64 is required for cloudInit type %s", vm.Spec.CloudInit.Type))
		}
		if vm.Spec.CloudInit.MetaDataBase64 == "" {
			return errors.New(fmt.Sprintf("metaDataBase64 is required for cloudInit type %s", vm.Spec.CloudInit.Type))
		}
		if vm.Spec.CloudInit.ConfigDiskTarget == "" {
			return errors.New(fmt.Sprintf("configDiskTarget is required for cloudInit type %s", vm.Spec.CloudInit.Type))
		}
	}

	return nil
}

func ApplyMetadata(vm *v1.VM) {
	if vm.Spec.CloudInit == nil {
		return
	}

	namespace := precond.MustNotBeEmpty(vm.GetObjectMeta().GetNamespace())
	domain := precond.MustNotBeEmpty(vm.GetObjectMeta().GetName())

	// TODO Put local-hostname in MetaData once we get pod DNS working with VMs
	msg := fmt.Sprintf("instance-id: %s-%s\n", namespace, domain)
	vm.Spec.CloudInit.MetaDataBase64 = base64.StdEncoding.EncodeToString([]byte(msg))
}

// This function is used to generate any containers related to cloud-init that
// need to be placed in the virt-launcher pod.
func GenerateContainers(vm *v1.VM, launcherImage string) ([]kubev1.Container, error) {
	var containers []kubev1.Container

	if vm.Spec.CloudInit == nil {
		return containers, nil
	}

	err := ValidateArgs(vm)
	if err != nil {
		return containers, err
	}

	switch vm.Spec.CloudInit.Type {
	case cloudInitConfigDisk:
		initialDelaySeconds := 5
		timeoutSeconds := 5
		periodSeconds := 10
		successThreshold := 2
		failureThreshold := 5

		path := getConfigDiskPath(vm)

		containers = append(containers, kubev1.Container{
			Name:            "config-disk",
			Image:           launcherImage,
			ImagePullPolicy: kubev1.PullIfNotPresent,
			Command:         []string{"/config-disk.sh"},
			Env: []kubev1.EnvVar{
				{
					Name:  "CONFIG_DISK_PATH",
					Value: getConfigDiskPath(vm),
				},
				{
					Name:  "CONFIG_DISK_FILE",
					Value: configDiskFile,
				},
				{
					Name:  "USER_DATA_BASE64",
					Value: vm.Spec.CloudInit.UserDataBase64,
				},
				{
					Name:  "META_DATA_BASE64",
					Value: vm.Spec.CloudInit.MetaDataBase64,
				},
			},
			VolumeMounts: []kubev1.VolumeMount{
				{
					Name:      "config-disk",
					MountPath: path,
				},
			},

			ReadinessProbe: &kubev1.Probe{
				Handler: kubev1.Handler{
					Exec: &kubev1.ExecAction{
						Command: []string{
							"cat",
							"/tmp/healthy",
						},
					},
				},
				InitialDelaySeconds: int32(initialDelaySeconds),
				PeriodSeconds:       int32(periodSeconds),
				TimeoutSeconds:      int32(timeoutSeconds),
				SuccessThreshold:    int32(successThreshold),
				FailureThreshold:    int32(failureThreshold),
			},
		})
	}

	return containers, nil
}

// This function is used to generate any pod volumes related to cloud-init that
// need to be placed in the virt-launcher pod.
func GenerateVolumes(vm *v1.VM) ([]kubev1.Volume, error) {
	var volumes []kubev1.Volume

	if vm.Spec.CloudInit == nil {
		return volumes, nil
	}

	err := ValidateArgs(vm)
	if err != nil {
		return volumes, err
	}

	switch vm.Spec.CloudInit.Type {
	case cloudInitConfigDisk:
		path := getConfigDiskPath(vm)
		volumes = append(volumes, kubev1.Volume{
			Name: "config-disk",
			VolumeSource: kubev1.VolumeSource{
				HostPath: &kubev1.HostPathVolumeSource{
					Path: path,
				},
			},
		})
	}

	return volumes, nil
}
