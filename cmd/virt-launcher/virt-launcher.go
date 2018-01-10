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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/libvirt/libvirt-go"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/watch"

	"kubevirt.io/kubevirt/pkg/api/v1"
	cloudinit "kubevirt.io/kubevirt/pkg/cloud-init"
	"kubevirt.io/kubevirt/pkg/log"
	registrydisk "kubevirt.io/kubevirt/pkg/registry-disk"
	virtlauncher "kubevirt.io/kubevirt/pkg/virt-launcher"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap"
	virtcli "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/cli"
	cmdserver "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/cmd-server"
	cmdclient "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/cmd-server/client"
	notifyclient "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/notify-server/client"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/util"
	watchdog "kubevirt.io/kubevirt/pkg/watchdog"
)

const defaultStartTimeout = 3 * time.Minute
const defaultWatchdogInterval = 5 * time.Second

func markReady(readinessFile string) {
	f, err := os.OpenFile(readinessFile, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	f.Close()
	log.Log.Info("Marked as ready")
}

func startCmdServer(socketPath string,
	domainConn virtcli.Connection,
	domainManager virtwrap.DomainManager) {

	err := os.RemoveAll(socketPath)
	if err != nil {
		log.Log.Reason(err).Error("Could not clean up virt-launcher cmd socket")
		panic(err)
	}

	err = os.MkdirAll(filepath.Dir(socketPath), 0755)
	if err != nil {
		log.Log.Reason(err).Error("Could not create directory for socket.")
		panic(err)
	}

	go func() {
		err := cmdserver.RunServer(socketPath, domainConn, domainManager)
		if err != nil {
			log.Log.Reason(err).Error("Failed to start virt-launcher cmd server")
			panic(err)
		}
	}()

}

func createLibvirtConnection() virtcli.Connection {
	libvirtUri := "qemu:///system"
	domainConn, err := virtcli.NewConnection(libvirtUri, "", "", 10*time.Second)
	if err != nil {
		panic(fmt.Sprintf("failed to connect to libvirtd: %v", err))
	}

	return domainConn
}

func startDomainEventMonitoring(virtShareDir string, domainConn virtcli.Connection, deleteNotificationSent chan watch.Event) {
	libvirt.EventRegisterDefaultImpl()

	go func() {
		for {
			if res := libvirt.EventRunDefaultImpl(); res != nil {
				log.Log.Reason(res).Error("Listening to libvirt events failed, retrying.")
				time.Sleep(time.Second)
			}
		}
	}()

	err := notifyclient.StartNotifier(virtShareDir, domainConn, deleteNotificationSent)
	if err != nil {
		panic(err)
	}

}

func startWatchdogTicker(watchdogFile string, watchdogInterval time.Duration, stopChan chan struct{}) {
	err := watchdog.WatchdogFileUpdate(watchdogFile)
	if err != nil {
		panic(err)
	}

	log.Log.Infof("Watchdog file created at %s", watchdogFile)

	go func() {

		ticker := time.NewTicker(watchdogInterval).C
		for {
			select {
			case <-stopChan:
				return
			case <-ticker:
				err := watchdog.WatchdogFileUpdate(watchdogFile)
				if err != nil {
					panic(err)
				}
			}
		}
	}()
}

func initializeDirs(virtShareDir string,
	ephemeralDiskDir string,
	namespace string,
	name string) {

	err := virtlauncher.InitializeSharedDirectories(virtShareDir)
	if err != nil {
		panic(err)
	}

	err = virtlauncher.InitializePrivateDirectories(filepath.Join("/var/run/kubevirt-private", namespace, name))
	if err != nil {
		panic(err)
	}

	err = cloudinit.SetLocalDirectory(ephemeralDiskDir + "/cloud-init-data")
	if err != nil {
		panic(err)
	}

	err = registrydisk.SetLocalDirectory(ephemeralDiskDir + "/registry-disk-data")
	if err != nil {
		panic(err)
	}
}

func main() {
	log.InitializeLogging("virt-launcher")
	qemuTimeout := flag.Duration("qemu-timeout", defaultStartTimeout, "Amount of time to wait for qemu")
	virtShareDir := flag.String("kubevirt-share-dir", "/var/run/kubevirt", "Shared directory between virt-handler and virt-launcher")
	ephemeralDiskDir := flag.String("ephemeral-disk-dir", "/var/run/libvirt/kubevirt-ephemeral-disk", "Base directory for ephemeral disk data")
	name := flag.String("name", "", "Name of the VM")
	namespace := flag.String("namespace", "", "Namespace of the VM")
	watchdogInterval := flag.Duration("watchdog-update-interval", defaultWatchdogInterval, "Interval at which watchdog file should be updated")
	readinessFile := flag.String("readiness-file", "/tmp/health", "Pod looks for tihs file to determine when virt-launcher is initialized")
	gracePeriodSeconds := flag.Int("grace-period-seconds", 30, "Grace period to observe before sending SIGTERM to vm process.")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	vm := v1.NewVMReferenceFromNameWithNS(*namespace, *name)

	// Initialize local and shared directories
	initializeDirs(*virtShareDir, *ephemeralDiskDir, *namespace, *name)

	// Start libvirtd, virtlogd, and establish libvirt connection
	stopChan := make(chan struct{})
	defer close(stopChan)

	util.StartLibvirt(stopChan)
	util.StartVirtlog(stopChan)

	domainConn := createLibvirtConnection()
	defer domainConn.Close()

	domainManager, err := virtwrap.NewLibvirtDomainManager(domainConn)
	if err != nil {
		panic(err)
	}

	// Start the virt-launcher command service.
	// Clients can use this service to tell virt-launcher
	// to start/stop virtual machines
	socketPath := cmdclient.SocketFromNamespaceName(*virtShareDir, *namespace, *name)
	startCmdServer(socketPath, domainConn, domainManager)

	watchdogFile := watchdog.WatchdogFileFromNamespaceName(*virtShareDir,
		*namespace,
		*name)
	startWatchdogTicker(watchdogFile, *watchdogInterval, stopChan)

	gracefulShutdownTriggerFile := virtlauncher.GracefulShutdownTriggerFromNamespaceName(*virtShareDir,
		*namespace,
		*name)
	err = virtlauncher.GracefulShutdownTriggerClear(gracefulShutdownTriggerFile)
	if err != nil {
		log.Log.Reason(err).Errorf("Error clearing shutdown trigger file %s.", gracefulShutdownTriggerFile)
		panic(err)
	}

	shutdownCallback := func(pid int) {
		err := domainManager.KillVM(vm)
		if err != nil {
			log.Log.Reason(err).Errorf("Unable to stop qemu with libvirt, falling back to SIGTERM")
			syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	mon := virtlauncher.NewProcessMonitor("qemu",
		gracefulShutdownTriggerFile,
		*gracePeriodSeconds,
		shutdownCallback)

	deleteNotificationSent := make(chan watch.Event, 10)
	startDomainEventMonitoring(*virtShareDir, domainConn, deleteNotificationSent)

	markReady(*readinessFile)
	mon.RunForever(*qemuTimeout)

	// This forces the delete event to go out to virt-handler.
	domainManager.KillVM(vm)

	log.Log.Info("Waiting on final notifications to be sent to virt-handler.")
	select {
	case <-deleteNotificationSent:
		log.Log.Info("Final Delete notification sent, exiting")
	}
}
