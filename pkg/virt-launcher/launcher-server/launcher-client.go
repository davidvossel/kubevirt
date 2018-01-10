package launcherserver

import (
	"encoding/json"
	"fmt"
	"net/rpc"

	k8sv1 "k8s.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/api/v1"
	"kubevirt.io/kubevirt/pkg/log"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/api"
)

type VirtLauncherClient struct {
	client *rpc.Client
}

func GetClient(socketPath string) (*VirtLauncherClient, error) {
	conn, err := rpc.Dial("unix", socketPath)
	if err != nil {
		log.Log.Reason(err).Errorf("client failed to connect to virt-launcher socket: %s", socketPath)
		return nil, err
	}

	return &VirtLauncherClient{client: conn}, nil
}

func (c *VirtLauncherClient) Close() {
	c.client.Close()
}

func (c *VirtLauncherClient) sendCmd(vm *v1.VirtualMachine, cmd string) error {

	vmJSON, err := json.Marshal(*vm)
	if err != nil {
		return err
	}

	args := &Args{
		VMJSON: string(vmJSON),
	}

	reply := &Reply{}

	c.client.Call(cmd, args, reply)
	if reply.Success != true {
		msg := fmt.Sprintf("sending command %s failed: %s", cmd, reply.Message)
		return fmt.Errorf(msg)
	}
	return nil
}

func (c *VirtLauncherClient) ShutdownVirtualMachine(vm *v1.VirtualMachine) error {
	return c.sendCmd(vm, "Launcher.Shutdown")
}
func (c *VirtLauncherClient) KillVirtualMachine(vm *v1.VirtualMachine) error {
	return c.sendCmd(vm, "Launcher.Kill")
}
func (c *VirtLauncherClient) ListDomains() ([]*api.Domain, error) {

	var list []*api.Domain
	args := &Args{}

	cmd := "Launcher.ListDomains"
	reply := &Reply{}

	c.client.Call(cmd, args, reply)
	if reply.Success != true {
		msg := fmt.Sprintf("sending command %s failed: %s", cmd, reply.Message)
		return list, fmt.Errorf(msg)
	}
	err := json.Unmarshal([]byte(reply.DomainListJSON), &list)
	return list, err

}
func (c *VirtLauncherClient) StartVirtualMachine(vm *v1.VirtualMachine, secrets map[string]*k8sv1.Secret) error {

	cmd := "Launcher.Start"

	vmJSON, err := json.Marshal(*vm)
	if err != nil {
		return err
	}
	secretJSON, err := json.Marshal(secrets)
	if err != nil {
		return err
	}

	args := &Args{
		VMJSON:          string(vmJSON),
		K8SecretMapJSON: string(secretJSON),
	}

	reply := &Reply{}

	c.client.Call(cmd, args, reply)
	if reply.Success != true {
		msg := fmt.Sprintf("sending command %s failed: %s", cmd, reply.Message)
		return fmt.Errorf(msg)
	}

	return nil
}
func (c *VirtLauncherClient) SyncSecret(vm *v1.VirtualMachine, usageType string, usageID string, secretValue string) error {
	vmJSON, err := json.Marshal(*vm)
	if err != nil {
		return err
	}
	args := &Args{
		VMJSON:          string(vmJSON),
		SecretUsageType: usageType,
		SecretUsageID:   usageID,
		SecretValue:     secretValue,
	}

	cmd := "Launcher.SyncSecret"
	reply := &Reply{}

	c.client.Call(cmd, args, reply)
	if reply.Success != true {
		msg := fmt.Sprintf("sending command %s failed: %s", cmd, reply.Message)
		return fmt.Errorf(msg)
	}
	return nil
}
