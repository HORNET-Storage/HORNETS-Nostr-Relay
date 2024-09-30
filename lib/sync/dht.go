package sync

import (
	"github.com/anacrolix/dht/v2"
	"log"
)

func DefaultDHTServer() *dht.Server {
	// start dht server
	config := dht.NewDefaultServerConfig()
	dhtServer, err := dht.NewServer(config)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Starting DHT bootstrap")
	_, err = dhtServer.Bootstrap()
	if err != nil {
		log.Fatal(err)
	}

	//Wait for nodes to be added to the routing table
	//for i := 0; i < 30; i++ {
	//	stats := dhtServer.Stats()
	//	log.Printf("DHT stats: %+v", stats)
	//	if stats.GoodNodes > 0 {
	//		break
	//	}
	//	time.Sleep(2 * time.Second)
	//}

	return dhtServer
}
