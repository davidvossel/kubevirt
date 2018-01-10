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

package notifyserver

import (
	"encoding/json"
	"net"
	"net/rpc"
	"os"

	"k8s.io/apimachinery/pkg/watch"

	"kubevirt.io/kubevirt/pkg/log"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/api"
)

type Notifier struct {
	EventChan chan watch.Event
}

type NotifyReply struct {
	Success bool
	Message string
}

type NotifyArgs struct {
	DOMAINJSON string
	EventType  string
}

func (s *Notifier) DomainEvent(args *NotifyArgs, reply *NotifyReply) error {
	reply.Success = true

	domain := &api.Domain{}
	err := json.Unmarshal([]byte(args.DOMAINJSON), domain)
	if err != nil {
		log.Log.Errorf("Failed to unmarshal domain json object")
		reply.Success = false
		reply.Message = err.Error()
		return nil
	}

	log.Log.Infof("Received Domain Event")
	if args.EventType == string(watch.Added) {
		s.EventChan <- watch.Event{Type: watch.Added, Object: domain}
	}
	if args.EventType == string(watch.Modified) {
		s.EventChan <- watch.Event{Type: watch.Modified, Object: domain}
	}
	if args.EventType == string(watch.Deleted) {
		s.EventChan <- watch.Event{Type: watch.Deleted, Object: domain}
	}
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

func Run(socket string, c chan watch.Event) error {
	rpcServer := rpc.NewServer()
	server := &Notifier{
		EventChan: c,
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
