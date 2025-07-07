package sync

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/anacrolix/dht/v2"
)

func DefaultDHTServer() *dht.Server {
	// start dht server
	config := dht.NewDefaultServerConfig()
	dhtServer, err := dht.NewServer(config)
	if err != nil {
		logging.Fatalf("Failed to create DHT server: %v", err)
	}

	logging.Info("Starting DHT bootstrap")
	_, err = dhtServer.Bootstrap()
	if err != nil {
		logging.Fatalf("Failed to bootstrap DHT: %v", err)
	}

	//Wait for nodes to be added to the routing table
	//for i := 0; i < 30; i++ {
	//	stats := dhtServer.Stats()
	//	logging.Info("DHT stats: %+v", stats)
	//	if stats.GoodNodes > 0 {
	//		break
	//	}
	//	time.Sleep(2 * time.Second)
	//}

	return dhtServer
}
