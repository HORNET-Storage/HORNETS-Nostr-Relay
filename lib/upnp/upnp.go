package upnp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/huin/goupnp/dcps/internetgateway2"
	"golang.org/x/sync/errgroup"
)

type RouterClient interface {
	AddPortMapping(string, uint16, string, uint16, string, bool, string, uint32) error
	GetExternalIPAddress() (string, error)
	DeletePortMapping(string, uint16, string) error
	LocalAddr() net.IP
}

type UPnPManager struct {
	client RouterClient
	mu     sync.Mutex
}

var (
	instance *UPnPManager
	once     sync.Once
)

func Init(ctx context.Context) (*UPnPManager, error) {
	var err error
	once.Do(func() {
		var client RouterClient
		client, err = discoverClient(ctx)
		if err != nil {
			return
		}
		instance = &UPnPManager{client: client}
	})
	return instance, err
}

func Get() *UPnPManager {
	return instance
}

func discoverClient(ctx context.Context) (RouterClient, error) {
	group, _ := errgroup.WithContext(ctx)

	var ip1 []*internetgateway2.WANIPConnection1
	var ip2 []*internetgateway2.WANIPConnection2
	var ppp []*internetgateway2.WANPPPConnection1

	group.Go(func() error {
		var err error
		ip1, _, err = internetgateway2.NewWANIPConnection1Clients()
		return err
	})
	group.Go(func() error {
		var err error
		ip2, _, err = internetgateway2.NewWANIPConnection2Clients()
		return err
	})
	group.Go(func() error {
		var err error
		ppp, _, err = internetgateway2.NewWANPPPConnection1Clients()
		return err
	})

	if err := group.Wait(); err != nil {
		return nil, err
	}

	switch {
	case len(ip2) == 1:
		return ip2[0], nil
	case len(ip1) == 1:
		return ip1[0], nil
	case len(ppp) == 1:
		return ppp[0], nil
	default:
		return nil, errors.New("no supported UPnP clients found")
	}
}

func (u *UPnPManager) ForwardPort(port uint16, description string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	ip := u.client.LocalAddr().String()
	for _, proto := range []string{"TCP", "UDP"} {
		if err := u.client.AddPortMapping("", port, proto, port, ip, true, description, 3600); err != nil {
			return fmt.Errorf("failed to forward %s port %d: %v", proto, port, err)
		}
	}
	return nil
}

func (u *UPnPManager) RemovePort(port uint16) {
	u.mu.Lock()
	defer u.mu.Unlock()

	for _, proto := range []string{"TCP", "UDP"} {
		_ = u.client.DeletePortMapping("", port, proto)
	}
}

func (u *UPnPManager) ExternalIP() (string, error) {
	return u.client.GetExternalIPAddress()
}
