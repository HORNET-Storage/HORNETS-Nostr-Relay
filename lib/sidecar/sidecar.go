package sidecar

import (
	"sync"

	"github.com/spf13/viper"

	hsClient "github.com/hornet-storage/hornets-hyperswarm/clients/go/hyperswarm"
)

var (
	manager     *hsClient.Manager
	managerOnce sync.Once
)

func init() {
	viper.SetDefault("sidecar.address", "127.0.0.1:9100")
	viper.SetDefault("sidecar.executable", "")
	viper.SetDefault("sidecar.mode", "persistent")
	viper.SetDefault("sidecar.logdir", "logs/sidecar")
}

// GetClient returns a shared sidecar RPC client. It connects to an existing
// sidecar or starts one if an executable path is configured. The connection
// is automatically re-established if lost.
func GetClient() (*hsClient.Client, error) {
	managerOnce.Do(func() {
		manager = hsClient.NewManager(hsClient.SidecarConfig{
			Address:        viper.GetString("sidecar.address"),
			ExecutablePath: viper.GetString("sidecar.executable"),
			Mode:           viper.GetString("sidecar.mode"),
			LogDir:         viper.GetString("sidecar.logdir"),
		})
	})
	return manager.Client()
}

// Close shuts down the shared sidecar manager. If the relay started the
// sidecar process, it will be stopped. If connecting to an external sidecar,
// only the connection is closed.
func Close() {
	if manager != nil {
		manager.Close()
	}
}
