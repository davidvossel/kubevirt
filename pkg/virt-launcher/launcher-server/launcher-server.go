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

package launcherserver

import (
	"encoding/json"
	"net"
	"net/rpc"
	"os"
	"strings"

	"github.com/libvirt/libvirt-go"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"kubevirt.io/kubevirt/pkg/api/v1"
	"kubevirt.io/kubevirt/pkg/log"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/api"
	virtcli "kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/cli"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/util"
)

type Launcher struct {
	name          string
	domainConn    virtcli.Connection
	domainManager virtwrap.DomainManager
}

type Reply struct {
	Success        bool
	Message        string
	DomainListJSON string
}

type Args struct {
	// used for domain management
	VMJSON          string
	K8SecretMapJSON string

	// used for syncing secrets
	SecretUsageType string
	SecretUsageID   string
	SecretValue     string
}

func getK8SecretsFromArgs(args *Args) (map[string]*k8sv1.Secret, error) {
	var secrets map[string]*k8sv1.Secret
	err := json.Unmarshal([]byte(args.K8SecretMapJSON), &secrets)
	if err != nil {
		log.Log.Reason(err).Errorf("Failed to unmarshal k8 secrents json object")
		return nil, err
	}
	return secrets, nil
}

func getVmFromArgs(args *Args) (*v1.VirtualMachine, error) {
	vm := &v1.VirtualMachine{}
	err := json.Unmarshal([]byte(args.VMJSON), vm)
	if err != nil {
		log.Log.Reason(err).Errorf("Failed to unmarshal vm json object")
		return nil, err
	}
	return vm, nil
}

func (s *Launcher) SyncSecret(args *Args, reply *Reply) error {
	reply.Success = true

	vm, err := getVmFromArgs(args)
	if err != nil {
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	err = s.domainManager.SyncVMSecret(vm,
		args.SecretUsageType,
		args.SecretUsageID,
		args.SecretValue)

	if err != nil {
		log.Log.Object(vm).Reason(err).Errorf("Failed to sync vm secrets")
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	log.Log.Object(vm).Info("Synced vm")
	return nil
}

func (s *Launcher) Start(args *Args, reply *Reply) error {
	reply.Success = true

	vm, err := getVmFromArgs(args)
	if err != nil {
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	secrets, err := getK8SecretsFromArgs(args)
	if err != nil {
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	_, err = s.domainManager.SyncVM(vm, secrets)
	if err != nil {
		log.Log.Object(vm).Reason(err).Errorf("Failed to sync vm")
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	log.Log.Object(vm).Info("Synced vm")
	return nil
}

func (s *Launcher) Kill(args *Args, reply *Reply) error {
	reply.Success = true

	vm, err := getVmFromArgs(args)
	if err != nil {
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	err = s.domainManager.KillVM(vm)
	if err != nil {
		log.Log.Object(vm).Reason(err).Errorf("Failed to kill vm")
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	log.Log.Object(vm).Info("Signaled vm kill")
	return nil
}

func (s *Launcher) Shutdown(args *Args, reply *Reply) error {
	reply.Success = true

	vm, err := getVmFromArgs(args)
	if err != nil {
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	err = s.domainManager.SignalShutdownVM(vm)
	if err != nil {
		log.Log.Object(vm).Reason(err).Errorf("Failed to signal shutdown for vm")
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	log.Log.Object(vm).Info("Signaled vm shutdown")
	return nil
}

// SplitVMNamespaceKey returns the namespace and name that is encoded in the
// domain name.
func splitVMNamespaceKey(domainName string) (namespace, name string) {
	splitName := strings.SplitN(domainName, "_", 2)
	if len(splitName) == 1 {
		return k8sv1.NamespaceDefault, splitName[0]
	}
	return splitName[0], splitName[1]
}

func newDomain(dom virtcli.VirDomain) (*api.Domain, error) {

	name, err := dom.GetName()
	if err != nil {
		return nil, err
	}
	namespace, name := splitVMNamespaceKey(name)

	domain := api.NewDomainReferenceFromName(namespace, name)
	domain.GetObjectMeta().SetUID(domain.Spec.Metadata.KubeVirt.UID)
	return domain, nil
}

var LifeCycleTranslationMap = map[libvirt.DomainState]api.LifeCycle{
	libvirt.DOMAIN_NOSTATE:     api.NoState,
	libvirt.DOMAIN_RUNNING:     api.Running,
	libvirt.DOMAIN_BLOCKED:     api.Blocked,
	libvirt.DOMAIN_PAUSED:      api.Paused,
	libvirt.DOMAIN_SHUTDOWN:    api.Shutdown,
	libvirt.DOMAIN_SHUTOFF:     api.Shutoff,
	libvirt.DOMAIN_CRASHED:     api.Crashed,
	libvirt.DOMAIN_PMSUSPENDED: api.PMSuspended,
}

var ShutdownReasonTranslationMap = map[libvirt.DomainShutdownReason]api.StateChangeReason{
	libvirt.DOMAIN_SHUTDOWN_UNKNOWN: api.ReasonUnknown,
	libvirt.DOMAIN_SHUTDOWN_USER:    api.ReasonUser,
}

var ShutoffReasonTranslationMap = map[libvirt.DomainShutoffReason]api.StateChangeReason{
	libvirt.DOMAIN_SHUTOFF_UNKNOWN:       api.ReasonUnknown,
	libvirt.DOMAIN_SHUTOFF_SHUTDOWN:      api.ReasonShutdown,
	libvirt.DOMAIN_SHUTOFF_DESTROYED:     api.ReasonDestroyed,
	libvirt.DOMAIN_SHUTOFF_CRASHED:       api.ReasonCrashed,
	libvirt.DOMAIN_SHUTOFF_MIGRATED:      api.ReasonMigrated,
	libvirt.DOMAIN_SHUTOFF_SAVED:         api.ReasonSaved,
	libvirt.DOMAIN_SHUTOFF_FAILED:        api.ReasonFailed,
	libvirt.DOMAIN_SHUTOFF_FROM_SNAPSHOT: api.ReasonFromSnapshot,
}

var CrashedReasonTranslationMap = map[libvirt.DomainCrashedReason]api.StateChangeReason{
	libvirt.DOMAIN_CRASHED_UNKNOWN:  api.ReasonUnknown,
	libvirt.DOMAIN_CRASHED_PANICKED: api.ReasonPanicked,
}

func convState(status libvirt.DomainState) api.LifeCycle {
	return LifeCycleTranslationMap[status]
}

func convReason(status libvirt.DomainState, reason int) api.StateChangeReason {
	switch status {
	case libvirt.DOMAIN_SHUTDOWN:
		return ShutdownReasonTranslationMap[libvirt.DomainShutdownReason(reason)]
	case libvirt.DOMAIN_SHUTOFF:
		return ShutoffReasonTranslationMap[libvirt.DomainShutoffReason(reason)]
	case libvirt.DOMAIN_CRASHED:
		return CrashedReasonTranslationMap[libvirt.DomainCrashedReason(reason)]
	default:
		return api.ReasonUnknown
	}
}

func (s *Launcher) ListDomains(args *Args, reply *Reply) error {

	doms, err := s.domainConn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE | libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	var list []*api.Domain
	for _, dom := range doms {
		domain, err := newDomain(dom)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			reply.Success = false
			reply.Message = err.Error()
			return nil
		}
		spec, err := util.GetDomainSpec(dom)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			reply.Success = false
			reply.Message = err.Error()
			return nil
		}
		domain.Spec = *spec
		status, reason, err := dom.GetState()
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			reply.Success = false
			reply.Message = err.Error()
			return nil
		}
		domain.SetState(convState(status), convReason(status, reason))
		list = append(list, domain)
	}

	domainListJSON, err := json.Marshal(list)
	if err != nil {
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}
	reply.DomainListJSON = string(domainListJSON)

	return nil
}

func createSocket(socketPath string) (net.Listener, error) {

	os.RemoveAll(socketPath)
	socket, err := net.Listen("unix", socketPath)

	if err != nil {
		log.Log.Reason(err).Error("failed to create a socket for dhcp service")
		return nil, err
	}
	return socket, nil
}

func Run(socket string, domainConn virtcli.Connection) error {
	domainManager, err := virtwrap.NewLibvirtDomainManager(domainConn)
	rpcServer := rpc.NewServer()
	server := &Launcher{
		name:          "virt-launcher",
		domainConn:    domainConn,
		domainManager: domainManager,
	}
	rpcServer.Register(server)
	sock, err := createSocket(socket)
	if err != nil {
		return err
	}

	defer func() {
		sock.Close()
		os.Remove(socket)
	}()
	rpcServer.Accept(sock)

	return nil
}
