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
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/pflag"

	"github.com/libvirt/libvirt-go"

	"kubevirt.io/kubevirt/pkg/log"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/cache"
	virtcli "kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/cli"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/isolation"
	virtlauncher "kubevirt.io/kubevirt/pkg/virt-launcher"
	launcherserver "kubevirt.io/kubevirt/pkg/virt-launcher/launcher-server"
	watchdog "kubevirt.io/kubevirt/pkg/watchdog"
)

const defaultStartTimeout = 3 * time.Minute
const defaultWatchdogInterval = 10 * time.Second

func markReady(readinessFile string) {
	f, err := os.OpenFile(readinessFile, os.O_RDONLY|os.O_CREATE, 0666)
	if err != nil {
		panic(err)
	}
	f.Close()
	log.Log.Info("Marked as ready")
}

func startLibvirt(stopChan chan struct{}) {
	// we spawn libvirt from virt-launcher in order to ensure the libvirtd+qemu process
	// doesn't exit until virt-launcher is ready for it to. Virt-launcher traps signals
	// to perform special shutdown logic. These processes need to live in the same
	// container.

	go func() {
		for {
			exitChan := make(chan struct{})
			cmd := exec.Command("/libvirtd.sh")

			err := cmd.Start()
			if err != nil {
				log.Log.Reason(err).Error("failed to start libvirtd")
				panic(err)
			}

			go func() {
				defer close(exitChan)
				cmd.Wait()
			}()

			select {
			case <-stopChan:
				cmd.Process.Kill()
				return
			case <-exitChan:
				log.Log.Errorf("libvirtd exited, restarting")
			}

			// this sleep is to avoid consumming all resources in the
			// event of a libvirtd crash loop.
			time.Sleep(time.Second)
		}
	}()
}

func startCmdServer(socketPath string, domainConn virtcli.Connection) {
	err := os.MkdirAll(filepath.Dir(socketPath), 0755)
	if err != nil {
		log.Log.Reason(err).Error("Could not create directory for socket.")
		panic(err)
	}

	if err := os.RemoveAll(socketPath); err != nil {
		log.Log.Reason(err).Error("Could not clean up virt-launcher cmd socket")
		panic(err)
	}

	go func() {
		err := launcherserver.Run(socketPath, domainConn)
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

func startDomainEventMonitoring(domainConn virtcli.Connection) {
	libvirt.EventRegisterDefaultImpl()

	go func() {
		for {
			if res := libvirt.EventRunDefaultImpl(); res != nil {
				log.Log.Reason(res).Error("Listening to libvirt events failed, retrying.")
				time.Sleep(time.Second)
			}
		}
	}()

	err := cache.StartNotifier(domainConn)
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

func main() {
	log.InitializeLogging("virt-launcher")
	qemuTimeout := flag.Duration("qemu-timeout", defaultStartTimeout, "Amount of time to wait for qemu")
	virtShareDir := flag.String("kubevirt-share-dir", "/var/run/kubevirt", "Shared directory between virt-handler and virt-launcher")
	name := flag.String("name", "", "Name of the VM")
	namespace := flag.String("namespace", "", "Namespace of the VM")
	watchdogInterval := flag.Duration("watchdog-update-interval", defaultWatchdogInterval, "Interval at which watchdog file should be updated")
	readinessFile := flag.String("readiness-file", "/tmp/health", "Pod looks for tihs file to determine when virt-launcher is initialized")
	gracePeriodSeconds := flag.Int("grace-period-seconds", 30, "Grace period to observe before sending SIGTERM to vm process.")
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	stopChan := make(chan struct{})
	defer close(stopChan)

	err := virtlauncher.InitializeSharedDirectories(*virtShareDir)
	if err != nil {
		panic(err)
	}

	err = virtlauncher.InitializePrivateDirectories(filepath.Join("/var/run/kubevirt-private", *namespace, *name))
	if err != nil {
		panic(err)
	}

	startLibvirt(stopChan)

	domainConn := createLibvirtConnection()
	defer domainConn.Close()

	startDomainEventMonitoring(domainConn)

	socketPath := isolation.SocketFromNamespaceName(*virtShareDir, *namespace, *name)
	startCmdServer(socketPath, domainConn)

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

	mon := virtlauncher.NewProcessMonitor("qemu", gracefulShutdownTriggerFile, *gracePeriodSeconds)

	markReady(*readinessFile)
	mon.RunForever(*qemuTimeout)

	// TODO ensure exit domain event is received before shutting down virt-launcher
}
