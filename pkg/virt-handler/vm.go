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

package virthandler

import (
	"encoding/json"
	goerror "errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeevatkm/go-model"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"kubevirt.io/kubevirt/pkg/api/v1"
	cloudinit "kubevirt.io/kubevirt/pkg/cloud-init"
	configdisk "kubevirt.io/kubevirt/pkg/config-disk"
	"kubevirt.io/kubevirt/pkg/controller"
	diskutils "kubevirt.io/kubevirt/pkg/ephemeral-disk-utils"
	"kubevirt.io/kubevirt/pkg/kubecli"
	"kubevirt.io/kubevirt/pkg/log"
	"kubevirt.io/kubevirt/pkg/precond"
	registrydisk "kubevirt.io/kubevirt/pkg/registry-disk"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/api"
	virtlauncher "kubevirt.io/kubevirt/pkg/virt-launcher"
	watchdog "kubevirt.io/kubevirt/pkg/watchdog"
)

type updateType int

const (
	// This means the VM object has been deleted from the cluster
	updateTypeDeletion updateType = iota
	// This means the VM's corresponding virt-launcher watchdog has expired
	updateTypeWatchdogExpire
	// This means something has triggered graceful shutdown of the VM
	updateTypeGracefulShutdown
	// This is a normal update that occurs during the VM launch flow
	updateTypeNormal
)

func (p updateType) String() string {
	switch p {
	case updateTypeDeletion:
		return "Deletion"
	case updateTypeWatchdogExpire:
		return "WatchdogExpire"
	case updateTypeGracefulShutdown:
		return "GracefulShutdown"
	case updateTypeNormal:
		return "NormalUpdate"
	}
	return "Unknown"
}

func NewVMController(lw cache.ListerWatcher,
	domainManager virtwrap.DomainManager,
	recorder record.EventRecorder,
	restClient rest.RESTClient,
	clientset kubecli.KubevirtClient,
	host string,
	configDiskClient configdisk.ConfigDiskClient,
	virtShareDir string,
	watchdogTimeoutSeconds int) (cache.Store, workqueue.RateLimitingInterface, *controller.Controller) {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	dispatch := NewVMHandlerDispatch(domainManager,
		recorder,
		&restClient,
		clientset,
		host,
		configDiskClient,
		virtShareDir,
		watchdogTimeoutSeconds)

	indexer, informer := controller.NewController(lw, queue, &v1.VirtualMachine{}, dispatch)
	return indexer, queue, informer

}
func NewVMHandlerDispatch(domainManager virtwrap.DomainManager,
	recorder record.EventRecorder,
	restClient *rest.RESTClient,
	clientset kubecli.KubevirtClient,
	host string,
	configDiskClient configdisk.ConfigDiskClient,
	virtShareDir string,
	watchdogTimeoutSeconds int) controller.ControllerDispatch {
	return &VMHandlerDispatch{
		domainManager:          domainManager,
		recorder:               recorder,
		restClient:             *restClient,
		clientset:              clientset,
		host:                   host,
		configDisk:             configDiskClient,
		virtShareDir:           virtShareDir,
		watchdogTimeoutSeconds: watchdogTimeoutSeconds,
	}
}

type VMHandlerDispatch struct {
	domainManager          virtwrap.DomainManager
	recorder               record.EventRecorder
	restClient             rest.RESTClient
	clientset              kubecli.KubevirtClient
	host                   string
	configDisk             configdisk.ConfigDiskClient
	virtShareDir           string
	watchdogTimeoutSeconds int
}

type gracePeriodInfo struct {
	GracePeriodSeconds       int64 `json:"GracePeriodSeconds"`
	GracePeriodStartTimeUnix int64 `json:"GracePeriodStartTimeUnix"`
}

func (d *VMHandlerDispatch) gracePeriodInfoFilePath(vm *v1.VirtualMachine) string {
	namespace := precond.MustNotBeEmpty(vm.GetObjectMeta().GetNamespace())
	domain := precond.MustNotBeEmpty(vm.GetObjectMeta().GetName())

	info := namespace + "_" + domain
	return filepath.Join(d.virtShareDir, "grace-period-info", info)
}

func (d *VMHandlerDispatch) initializeGracePeriodInfo(vm *v1.VirtualMachine) error {
	gracePeriodSeconds := int64(v1.DefaultGracePeriodSeconds)
	if vm.Spec.TerminationGracePeriodSeconds != nil {
		gracePeriodSeconds = *vm.Spec.TerminationGracePeriodSeconds
	}

	info := &gracePeriodInfo{
		GracePeriodSeconds:       gracePeriodSeconds,
		GracePeriodStartTimeUnix: 0,
	}
	return d.setGracePeriodInfo(vm, info)
}

func (d *VMHandlerDispatch) signalGracePeriodStarted(vm *v1.VirtualMachine, ignoreNonExistant bool) error {
	now := time.Now().UTC().Unix()

	info, err := d.getGracePeriodInfo(vm)
	if err != nil {
		return err
	} else if info == nil {
		if ignoreNonExistant == false {
			gracePeriodSeconds := int64(v1.DefaultGracePeriodSeconds)
			if vm.Spec.TerminationGracePeriodSeconds != nil {
				gracePeriodSeconds = *vm.Spec.TerminationGracePeriodSeconds
			}
			info := &gracePeriodInfo{
				GracePeriodSeconds:       gracePeriodSeconds,
				GracePeriodStartTimeUnix: now,
			}
			return d.setGracePeriodInfo(vm, info)
		} else {
			return nil
		}
	}

	// only update start time if it was not previously set
	if info.GracePeriodStartTimeUnix == 0 {
		info.GracePeriodStartTimeUnix = time.Now().UTC().Unix()
		return d.setGracePeriodInfo(vm, info)
	}

	return nil
}

func (d *VMHandlerDispatch) hasGracePeriodExpired(vm *v1.VirtualMachine) (bool, error) {
	info, err := d.getGracePeriodInfo(vm)

	if err != nil {
		return false, err
	} else if info == nil {
		return true, err
	}

	if info.GracePeriodStartTimeUnix == 0 {
		return false, nil
	}

	now := time.Now().UTC().Unix()
	diff := now - info.GracePeriodStartTimeUnix

	if diff >= info.GracePeriodSeconds {
		return true, nil
	}
	return false, nil
}

func (d *VMHandlerDispatch) setGracePeriodInfo(vm *v1.VirtualMachine, info *gracePeriodInfo) error {
	path := d.gracePeriodInfoFilePath(vm)

	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return err
	}

	buf, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path, buf, 0644)
	if err != nil {
		return err
	}
	return nil
}

func (d *VMHandlerDispatch) getGracePeriodInfo(vm *v1.VirtualMachine) (*gracePeriodInfo, error) {
	path := d.gracePeriodInfoFilePath(vm)
	exists, err := diskutils.FileExists(path)
	if err != nil {
		return nil, err
	} else if !exists {
		return nil, nil
	}

	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	info := gracePeriodInfo{}
	err = json.Unmarshal(buf, &info)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (d *VMHandlerDispatch) deleteGracePeriodInfo(vm *v1.VirtualMachine) error {
	path := d.gracePeriodInfoFilePath(vm)
	return diskutils.RemoveFile(path)
}

func (d *VMHandlerDispatch) getVMNodeAddress(vm *v1.VirtualMachine) (string, error) {
	node, err := d.clientset.CoreV1().Nodes().Get(vm.Status.NodeName, metav1.GetOptions{})
	if err != nil {
		log.Log.Reason(err).Errorf("fetching source node %s failed", vm.Status.NodeName)
		return "", err
	}

	addrStr := ""
	for _, addr := range node.Status.Addresses {
		if (addr.Type == k8sv1.NodeInternalIP) && (addrStr == "") {
			addrStr = addr.Address
			break
		}
	}
	if addrStr == "" {
		err := fmt.Errorf("VM node is unreachable")
		log.Log.Error("VM node is unreachable")
		return "", err
	}

	return addrStr, nil
}

func (d *VMHandlerDispatch) updateVMStatus(vm *v1.VirtualMachine, cfg *api.DomainSpec) error {
	obj, err := scheme.Scheme.Copy(vm)
	if err != nil {
		return err
	}
	vm = obj.(*v1.VirtualMachine)

	// XXX When we start supporting hotplug, this needs to be altered.
	// Check if the VM is already marked as running. If yes, don't update the VM.
	// Otherwise we end up in endless controller requeues.
	if vm.Status.Phase == v1.Running {
		return nil
	}

	vm.Status.Phase = v1.Running

	vm.Status.Graphics = []v1.VMGraphics{}

	podIP, err := d.getVMNodeAddress(vm)
	if err != nil {
		return err
	}

	for _, src := range cfg.Devices.Graphics {
		if (src.Type != "spice" && src.Type != "vnc") || src.Port == -1 {
			continue
		}
		dst := v1.VMGraphics{
			Type: src.Type,
			Host: podIP,
			Port: src.Port,
		}
		vm.Status.Graphics = append(vm.Status.Graphics, dst)
	}

	return d.restClient.Put().Resource("virtualmachines").Body(vm).
		Name(vm.ObjectMeta.Name).Namespace(vm.ObjectMeta.Namespace).Do().Error()

}

func (d *VMHandlerDispatch) Execute(store cache.Store, queue workqueue.RateLimitingInterface, key interface{}) {

	processUpdateType := updateTypeNormal

	// Fetch the latest Vm state from cache
	obj, exists, err := store.GetByKey(key.(string))

	if err != nil {
		queue.AddRateLimited(key)
		return
	}

	// Retrieve the VM
	var vm *v1.VirtualMachine
	if !exists {
		namespace, name, err := cache.SplitMetaNamespaceKey(key.(string))
		if err != nil {
			// TODO do something more smart here
			queue.AddRateLimited(key)
			return
		}
		vm = v1.NewVMReferenceFromNameWithNS(namespace, name)
	} else {
		vm = obj.(*v1.VirtualMachine)
	}

	// Determine if the VM should be processed as deleted
	if !exists {
		// Check For Migration before processing vm not in our cache
		// If we don't have the VM in the cache, it could be that it is currently migrating to us
		isDestination, err := d.isMigrationDestination(vm.GetObjectMeta().GetNamespace(), vm.GetObjectMeta().GetName())
		if err != nil {
			// unable to determine migration status, we'll try again later.
			queue.AddRateLimited(key)
			return
		}

		if isDestination {
			// OK, this VM is migrating to us, don't interrupt it.
			queue.Forget(key)
			return
		}

		// The VM is deleted on the cluster, continue with processing the deletion on the host.
		processUpdateType = updateTypeDeletion
	}

	// Determine if VM's watchdog has expired
	watchdogExpired, err := watchdog.WatchdogFileIsExpired(d.watchdogTimeoutSeconds, d.virtShareDir, vm)
	if watchdogExpired {
		if vm.IsRunning() {
			log.Log.V(2).Object(vm).Info("Detected expired watchdog file for running VM.")
			processUpdateType = updateTypeWatchdogExpire
		} else if vm.IsFinal() {
			err = watchdog.WatchdogFileRemove(d.virtShareDir, vm)
			if err != nil {
				queue.AddRateLimited(key)
				return
			}
		}
	}

	// Determine if VM needs to be gracefully shutdown
	if exists && vm.IsRunning() {
		gracefulShutdown, err := virtlauncher.VmHasGracefulShutdownTrigger(d.virtShareDir, vm)
		if err != nil {
			queue.AddRateLimited(key)
			return
		}
		if gracefulShutdown && vm.IsRunning() {
			processUpdateType = updateTypeGracefulShutdown
		}
	}

	log.Log.Object(vm).V(3).Infof("Processing VM update of type %s", processUpdateType.String())

	isPending := false
	// Process the VM
	switch processUpdateType {
	case updateTypeDeletion:
		isPending, err = d.handleDeletion(vm)
	case updateTypeWatchdogExpire:
		isPending, err = d.handleWatchdogExpire(vm)
	case updateTypeGracefulShutdown:
		isPending, err = d.handleGracefulShutdown(vm)
	case updateTypeNormal:
		isPending, err = d.handleNormalUpdate(vm)
	}

	if err != nil {
		// Something went wrong, reenqueue the item with a delay
		log.Log.Object(vm).Reason(err).Error("Synchronizing the VM failed.")
		d.recorder.Event(vm, k8sv1.EventTypeWarning, v1.SyncFailed.String(), err.Error())
		queue.AddRateLimited(key)
		return
	} else if isPending {
		// waiting on an async action to complete
		log.Log.Object(vm).V(3).Info("Synchronizing is in a pending state.")
		queue.AddAfter(key, 1*time.Second)
		queue.Forget(key)
		return
	}

	log.Log.Object(vm).V(3).Info("Synchronizing the VM succeeded.")
	queue.Forget(key)
	return
}

// Almost everything in the VM object maps exactly to its domain counterpart
// One exception is persistent volume claims. This function looks up each PV
// and inserts a corrected disk entry into the VM's device map.
func MapPersistentVolumes(vm *v1.VirtualMachine, restClient cache.Getter, namespace string) (*v1.VirtualMachine, error) {
	vmCopy := &v1.VirtualMachine{}
	model.Copy(vmCopy, vm)
	logger := log.Log.Object(vm)

	for idx, disk := range vmCopy.Spec.Domain.Devices.Disks {
		if disk.Type == "PersistentVolumeClaim" {
			logger.V(3).Infof("Mapping PersistentVolumeClaim: %s", disk.Source.Name)

			// Look up existing persistent volume
			obj, err := restClient.Get().Namespace(namespace).Resource("persistentvolumeclaims").Name(disk.Source.Name).Do().Get()

			if err != nil {
				logger.Reason(err).Error("unable to look up persistent volume claim")
				return vm, fmt.Errorf("unable to look up persistent volume claim: %v", err)
			}

			pvc := obj.(*k8sv1.PersistentVolumeClaim)
			if pvc.Status.Phase != k8sv1.ClaimBound {
				logger.Error("attempted use of unbound persistent volume")
				return vm, fmt.Errorf("attempted use of unbound persistent volume claim: %s", pvc.Name)
			}

			// Look up the PersistentVolume this PVC is bound to
			// Note: This call is not namespaced!
			obj, err = restClient.Get().Resource("persistentvolumes").Name(pvc.Spec.VolumeName).Do().Get()

			if err != nil {
				logger.Reason(err).Error("unable to access persistent volume record")
				return vm, fmt.Errorf("unable to access persistent volume record: %v", err)
			}
			pv := obj.(*k8sv1.PersistentVolume)

			logger.Infof("Mapping PVC %s", pv.Name)
			newDisk, err := mapPVToDisk(&disk, pv)

			if err != nil {
				logger.Reason(err).Errorf("Mapping PVC %s failed", pv.Name)
				return vm, err
			}

			vmCopy.Spec.Domain.Devices.Disks[idx] = *newDisk
		} else if disk.Type == "network" {
			newDisk := v1.Disk{}
			model.Copy(&newDisk, disk)

			if disk.Source.Host == nil {
				logger.Error("Missing disk source host")
				return vm, fmt.Errorf("Missing disk source host")
			}

			ipAddrs, err := net.LookupIP(disk.Source.Host.Name)
			if err != nil || ipAddrs == nil || len(ipAddrs) < 1 {
				logger.Reason(err).Errorf("Unable to resolve host '%s'", disk.Source.Host.Name)
				return vm, fmt.Errorf("Unable to resolve host '%s': %s", disk.Source.Host.Name, err)
			}

			newDisk.Source.Host.Name = ipAddrs[0].String()

			vmCopy.Spec.Domain.Devices.Disks[idx] = newDisk
		}
	}

	return vmCopy, nil
}

func mapPVToDisk(disk *v1.Disk, pv *k8sv1.PersistentVolume) (*v1.Disk, error) {
	if pv.Spec.ISCSI != nil {
		newDisk := v1.Disk{}

		newDisk.Type = "network"
		newDisk.Device = "disk"
		newDisk.Target = disk.Target
		newDisk.Driver = new(v1.DiskDriver)
		newDisk.Driver.Type = "raw"
		newDisk.Driver.Name = "qemu"

		newDisk.Source.Name = fmt.Sprintf("%s/%d", pv.Spec.ISCSI.IQN, pv.Spec.ISCSI.Lun)
		newDisk.Source.Protocol = "iscsi"

		hostPort := strings.Split(pv.Spec.ISCSI.TargetPortal, ":")
		ipAddrs, err := net.LookupIP(hostPort[0])
		if err != nil || len(ipAddrs) < 1 {
			return nil, fmt.Errorf("Unable to resolve host '%s': %s", hostPort[0], err)
		}

		newDisk.Source.Host = &v1.DiskSourceHost{}
		newDisk.Source.Host.Name = ipAddrs[0].String()
		if len(hostPort) > 1 {
			newDisk.Source.Host.Port = hostPort[1]
		}

		// This iscsi device has auth associated with it.
		if pv.Spec.ISCSI.SecretRef != nil && pv.Spec.ISCSI.SecretRef.Name != "" {
			newDisk.Auth = &v1.DiskAuth{
				Secret: &v1.DiskSecret{
					Type:  "iscsi",
					Usage: pv.Spec.ISCSI.SecretRef.Name,
				},
			}
		}
		return &newDisk, nil
	} else {
		err := fmt.Errorf("Referenced PV %s is backed by an unsupported storage type. Only iSCSI is supported.", pv.ObjectMeta.Name)
		return nil, err
	}
}

func (d *VMHandlerDispatch) injectDiskAuth(vm *v1.VirtualMachine) (*v1.VirtualMachine, error) {
	for idx, disk := range vm.Spec.Domain.Devices.Disks {
		if disk.Auth == nil || disk.Auth.Secret == nil || disk.Auth.Secret.Usage == "" {
			continue
		}

		usageIDSuffix := fmt.Sprintf("-%s-%s---", vm.GetObjectMeta().GetNamespace(), vm.GetObjectMeta().GetName())
		usageID := disk.Auth.Secret.Usage
		usageType := disk.Auth.Secret.Type
		secretID := usageID

		if strings.HasSuffix(usageID, usageIDSuffix) {
			secretID = strings.TrimSuffix(usageID, usageIDSuffix)
		} else {
			usageID = fmt.Sprintf("%s%s", usageID, usageIDSuffix)
		}

		secret, err := d.clientset.CoreV1().Secrets(vm.ObjectMeta.Namespace).Get(secretID, metav1.GetOptions{})
		if err != nil {
			log.Log.Reason(err).Error("Defining the VM secret failed unable to pull corresponding k8s secret value")
			return nil, err
		}

		secretValue, ok := secret.Data["node.session.auth.password"]
		if ok == false {
			return nil, goerror.New(fmt.Sprintf("No password value found in k8s secret %s %v", secretID, err))
		}

		userValue, ok := secret.Data["node.session.auth.username"]
		if ok == false {
			return nil, goerror.New(fmt.Sprintf("Failed to find username for disk auth %s", secretID))
		}
		vm.Spec.Domain.Devices.Disks[idx].Auth.Username = string(userValue)

		// override the usage id on the VM with the VM specific one.
		// By decoupling usage from the k8s secret name here, this allows
		// multiple VMs to reference the same k8s secret without conflicting
		// with one another.
		vm.Spec.Domain.Devices.Disks[idx].Auth.Secret.Usage = usageID

		err = d.domainManager.SyncVMSecret(vm, usageType, usageID, string(secretValue))
		if err != nil {
			return nil, err
		}
	}

	return vm, nil
}

func (d *VMHandlerDispatch) handleDeletion(vm *v1.VirtualMachine) (bool, error) {

	err := d.signalGracePeriodStarted(vm, true)
	if err != nil {
		return false, err
	}

	expired, err := d.hasGracePeriodExpired(vm)
	if err != nil {
		return false, err
	}

	if expired == false {
		err = d.domainManager.SignalShutdownVM(vm)
		if err != nil {
			return false, err
		}
		// pending graceful shutdown.
		return true, nil
	}

	log.Log.Object(vm).Infof("grace period expired, killing deleted VM %s", vm.GetObjectMeta().GetName())

	// Since the VM was not in the cache, we delete it
	err = d.domainManager.KillVM(vm)
	if err != nil {
		return false, err
	}

	// remove any defined libvirt secrets associated with this vm
	err = d.domainManager.RemoveVMSecrets(vm)
	if err != nil {
		return false, err
	}

	err = registrydisk.CleanupEphemeralDisks(vm)
	if err != nil {
		return false, err
	}

	err = watchdog.WatchdogFileRemove(d.virtShareDir, vm)
	if err != nil {
		return false, err
	}

	err = virtlauncher.VmGracefulShutdownTriggerClear(d.virtShareDir, vm)
	if err != nil {
		return false, err
	}

	err = d.deleteGracePeriodInfo(vm)
	if err != nil {
		return false, err
	}

	return false, d.configDisk.Undefine(vm)

}

func (d *VMHandlerDispatch) handleWatchdogExpire(vm *v1.VirtualMachine) (bool, error) {
	log.Log.Object(vm).Info("Processing expired watchdog")

	err := d.domainManager.KillVM(vm)
	if err != nil {
		return false, err
	}

	return false, nil
}

func (d *VMHandlerDispatch) handleGracefulShutdown(vm *v1.VirtualMachine) (bool, error) {
	log.Log.Object(vm).Info("Requesting shutdown due to graceful shutdown trigger")

	err := d.signalGracePeriodStarted(vm, false)
	if err != nil {
		return false, err
	}

	expired, err := d.hasGracePeriodExpired(vm)
	if err != nil {
		return false, err
	}

	if expired {
		log.Log.Object(vm).Infof("grace period expired, killing VM %s", vm.GetObjectMeta().GetName())
		err := d.domainManager.KillVM(vm)
		if err != nil {
			return false, err
		}

		return false, nil
	}

	err = d.domainManager.SignalShutdownVM(vm)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *VMHandlerDispatch) handleNormalUpdate(vm *v1.VirtualMachine) (bool, error) {
	if isWorthSyncing(vm) == false {
		// nothing to do here.
		return false, nil
	}

	hasWatchdog, err := watchdog.WatchdogFileExists(d.virtShareDir, vm)
	if err != nil {
		log.Log.Object(vm).Reason(err).V(3).Error("Error accessing virt-launcher watchdog file.")
		return false, err
	}
	if hasWatchdog == false {
		log.Log.Object(vm).Reason(err).V(3).Error("Could not detect virt-launcher watchdog file.")
		return false, goerror.New(fmt.Sprintf("No watchdog file found for vm"))
	}

	isPending, err := d.configDisk.Define(vm)
	if err != nil || isPending == true {
		return isPending, err
	}

	// Synchronize the VM state
	vm, err = MapPersistentVolumes(vm, d.clientset.CoreV1().RESTClient(), vm.ObjectMeta.Namespace)
	if err != nil {
		return false, err
	}

	// Map Container Registry Disks to block devices Libvirt can consume
	vm, err = registrydisk.MapRegistryDisks(vm)
	if err != nil {
		return false, err
	}

	vm, err = d.injectDiskAuth(vm)
	if err != nil {
		return false, err
	}

	// Map whatever devices are being used for config-init
	vm, err = cloudinit.MapCloudInitDisks(vm)
	if err != nil {
		return false, err
	}

	// TODO MigrationNodeName should be a pointer
	if vm.Status.MigrationNodeName != "" {
		// Only sync if the VM is not marked as migrating.
		// Everything except shutting down the VM is not
		// permitted when it is migrating.
		return false, nil
	}

	// store grace period info locally in case VM object is deleted before shutting down
	err = d.initializeGracePeriodInfo(vm)
	if err != nil {
		return false, err
	}

	// TODO check if found VM has the same UID like the domain,
	// if not, delete the Domain first
	newCfg, err := d.domainManager.SyncVM(vm)
	if err != nil {
		return false, err
	}

	err = d.updateVMStatus(vm, newCfg)
	if err != nil {
		return false, err
	}

	return false, nil
}

func (d *VMHandlerDispatch) isMigrationDestination(namespace string, vmName string) (bool, error) {

	// If we don't have the VM in the cache, it could be that it is currently migrating to us
	result := d.restClient.Get().Name(vmName).Resource("virtualmachines").Namespace(namespace).Do()
	if result.Error() == nil {
		// So the VM still seems to exist
		fetchedVM, err := result.Get()
		if err != nil {
			return false, err
		}
		if fetchedVM.(*v1.VirtualMachine).Status.MigrationNodeName == d.host {
			return true, nil
		}
	} else if !errors.IsNotFound(result.Error()) {
		// Something went wrong, let's try again later
		return false, result.Error()
	}

	// VM object was not found.
	return false, nil
}

func isWorthSyncing(vm *v1.VirtualMachine) bool {
	return vm.Status.Phase != v1.Succeeded && vm.Status.Phase != v1.Failed
}
