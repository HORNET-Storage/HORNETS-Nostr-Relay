package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/filter"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind0"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind3"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30023"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind5"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind6"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind7"
	universalhandler "github.com/HORNET-Storage/hornet-storage/lib/handlers/universal"
	"github.com/HORNET-Storage/hornet-storage/lib/proxy"
	"github.com/HORNET-Storage/hornet-storage/lib/web"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	//"github.com/libp2p/go-libp2p/p2p/security/noise"
	//libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	//stores_bbolt "github.com/HORNET-Storage/hornet-storage/lib/stores/bbolt"
	//stores_memory "github.com/HORNET-Storage/hornet-storage/lib/stores/memory"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
)

func init() {
	viper.SetDefault("key", "")
	viper.SetDefault("web", false)
	viper.SetDefault("proxy", true)
	viper.SetDefault("port", "9000")

	viper.AddConfigPath(".")
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
	})

	viper.WatchConfig()
}

func main() {
	wg := new(sync.WaitGroup)

	// Private key
	key := strings.TrimPrefix(viper.GetString("key"), "nsec")

	decodedKey, err := hex.DecodeString(key)
	if err != nil {
		log.Fatal(err)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(decodedKey)
	if err != nil {
		log.Fatal(err)
	}

	// Create and initialize database
	store := &stores_graviton.GravitonStore{}

	store.InitStore()

	// Libp2p Host
	listenAddress := fmt.Sprintf("/ip4/127.0.0.1/udp/%s/quic-v1", viper.GetString("port"))

	host, err := libp2p.New(
		libp2p.Identity(privateKey),
		// Multiple listen addresses
		libp2p.ListenAddrStrings(
			listenAddress,
		),
		// support TLS connections
		//libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support noise connections
		//libp2p.Security(noise.ID, noise.New),

		//libp2p.Transport(customQUICConstructor),
		// support any other default transports (TCP)
		//libp2p.DefaultTransports,
		libp2p.Transport(libp2pquic.NewTransport),
		//libp2p.Transport(transport),
		// Let's prevent our peer from having too many
		// connections by attaching a connection manager.

		//libp2p.ConnectionManager(connmgr),
		// Attempt to open ports using uPNP for NATed hosts.

		//libp2p.NATPortMap(),
		// Let this host use the DHT to find other hosts

		//libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
		//	idht, err = dht.New(ctx, h)
		//	return idht, err
		//}),

		// If you want to help other peers to figure out if they are behind
		// NATs, you can launch the server-side of AutoNAT too (AutoRelay
		// already runs the client)
		//
		// This service is highly rate-limited and should not cause any
		// performance issues.

		//libp2p.EnableNATService(),
	)

	if err != nil {
		log.Fatal(err)
	}

	// Stream Handlers
	handlers.AddDownloadHandler(host, store, func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		// Check keys or potential future permissions here

		return true
	})

	handlers.AddUploadHandler(host, store, func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		// Check keys or potential future permissions here

		return true
	}, func(dag *merkle_dag.Dag, pubKey *string) {
		// Don't need to do anything here right now
	})

	// Register Our Nostr Stream Handlers
	nostr.RegisterHandler("universal", universalhandler.BuildUniversalHandler(store))
	nostr.RegisterHandler("kind/0", kind0.BuildKind0Handler(store))
	nostr.RegisterHandler("kind/1", kind1.BuildKind1Handler(store))
	nostr.RegisterHandler("kind/3", kind3.BuildKind3Handler(store))
	nostr.RegisterHandler("kind/5", kind5.BuildKind5Handler(store))
	nostr.RegisterHandler("kind/6", kind6.BuildKind6Handler(store))
	nostr.RegisterHandler("kind/7", kind7.BuildKind7Handler(store))
	nostr.RegisterHandler("kind/30023", kind30023.BuildKind30023Handler(store))
	nostr.RegisterHandler("filter", filter.BuildFilterHandler(store))

	// Register a libp2p handler for every stream handler
	for kind, handler := range nostr.GetHandlers() {
		wrapper := func(stream network.Stream) {
			read := func() ([]byte, error) {
				return io.ReadAll(stream)
			}

			write := func(messageType string, params ...interface{}) {
				response := nostr.BuildResponse(messageType, params)

				if len(response) > 0 {
					stream.Write(response)
				}
			}

			handler(read, write)

			stream.Close()
		}

		host.SetStreamHandler(protocol.ID("/nostr/event/"+kind), wrapper)
	}

	// Web Panel
	if viper.GetBool("web") {
		wg.Add(1)

		fmt.Println("Starting with web server enabled")

		go func() {
			err := web.StartServer()

			if err != nil {
				fmt.Println("Fatal error occured in web server")
			}

			wg.Done()
		}()
	}

	// Proxy web sockets
	if viper.GetBool("proxy") {
		wg.Add(1)

		fmt.Println("Starting with legacy nostr proxy web server enabled")

		go func() {
			err := proxy.StartServer()

			if err != nil {
				fmt.Println("Fatal error occured in web server")
			}

			wg.Done()
		}()
	}

	defer host.Close()

	fmt.Printf("Host started with id: %s\n", host.ID())

	wg.Wait()
}
