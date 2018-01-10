package notifyserver

import (
	"encoding/json"
	"fmt"
	"net/rpc"

	"k8s.io/apimachinery/pkg/watch"

	"kubevirt.io/kubevirt/pkg/log"
	"kubevirt.io/kubevirt/pkg/virt-handler/virtwrap/api"
)

type DomainNotifierClient struct {
	client *rpc.Client
}

func NewDomainNotifierClient(socketPath string) (*DomainNotifierClient, error) {
	conn, err := rpc.Dial("unix", socketPath)
	if err != nil {
		log.Log.Reason(err).Errorf("client failed to connect to domain notifier socket: %s", socketPath)
		return nil, err
	}

	return &DomainNotifierClient{client: conn}, nil
}

func (c *DomainNotifierClient) DomainEvent(event watch.Event) error {

	domain := event.Object.(*api.Domain)
	domainJSON, err := json.Marshal(domain)
	if err != nil {
		return err
	}

	args := &NotifyArgs{
		DOMAINJSON: string(domainJSON),
		EventType:  string(event.Type),
	}

	reply := &NotifyReply{}

	c.client.Call("Notifier.DomainEvent", args, reply)
	if reply.Success != true {
		msg := fmt.Sprintf("failed to notify domain event: %s", reply.Message)
		return fmt.Errorf(msg)
	}

	return nil
}
