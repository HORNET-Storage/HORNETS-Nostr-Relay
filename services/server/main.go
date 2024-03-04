package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/web"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"

	keys "github.com/HORNET-Storage/hornet-storage/lib/context"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	//stores_bbolt "github.com/HORNET-Storage/hornet-storage/lib/stores/bbolt"
	//stores_memory "github.com/HORNET-Storage/hornet-storage/lib/stores/memory"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
)

const DefaultPort = "9000"

func main() {
	ctx := context.Background()

	wg := new(sync.WaitGroup)

	keyFlag := flag.String("key", "", "Private key used to identify this node")
	webFlag := flag.Bool("web", false, "Launch web server: true/false")
	portFlag := flag.String("port", "", "Port to run the node on")

	flag.Parse()

	// Web Panel
	if *webFlag {
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

	// Port
	port := DefaultPort
	if portFlag != nil {
		port = *portFlag
	}

	// Private key
	var priv crypto.PrivKey
	if keyFlag != nil {
		bytes, err := hex.DecodeString(*keyFlag)
		if err != nil {
			log.Fatal(err)
		}

		privateKey, err := crypto.UnmarshalPrivateKey(bytes)
		if err != nil {
			log.Fatal(err)
		}

		priv = privateKey
	} else {
		privateKey, _, err := crypto.GenerateKeyPair(
			crypto.Ed25519,
			-1,
		)
		if err != nil {
			log.Fatal(err)
		}

		priv = privateKey
	}

	// New storage implementation (will replace the above)
	store := &stores_graviton.GravitonStore{}

	store.InitStore()

	ctx = context.WithValue(ctx, keys.Storage, store)

	// This works but feels weird, open to better solutions
	//defer store.UserDatabase.Db.Close()
	//defer store.ContentDatabase.Db.Close()

	// Setup libp2p Connection Manager
	connmgr, err := connmgr.NewConnManager(
		100, // Lowwater
		400, // HighWater,
		connmgr.WithGracePeriod(time.Minute),
	)
	if err != nil {
		panic(err)
	}

	// Libp2p Host
	listenAddress := fmt.Sprintf("/ip4/0.0.0.0/tcp/%s", port)

	host, err := libp2p.New(
		libp2p.Identity(priv),
		// Multiple listen addresses
		libp2p.ListenAddrStrings(
			listenAddress,
		),
		// support TLS connections
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support noise connections
		libp2p.Security(noise.ID, noise.New),

		//libp2p.Transport(customQUICConstructor),
		// support any other default transports (TCP)
		libp2p.DefaultTransports,
		//libp2p.Transport(transport),
		// Let's prevent our peer from having too many
		// connections by attaching a connection manager.

		libp2p.ConnectionManager(connmgr),
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

	log.Println("Adding download handler")

	// Stream Handlers
	handlers.AddDownloadHandler(host, store, func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		// Check keys or potential future permissions here

		return true
	})

	log.Println("Adding upload handler")

	handlers.AddUploadHandler(host, store, func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		// Check keys or potential future permissions here

		return true
	}, func(dag *merkle_dag.Dag, pubKey *string) {
		// Don't need to do anything here right now
	})

	defer host.Close()

	ctx = context.WithValue(ctx, keys.Host, host)

	fmt.Printf("Host started with id: %s\n", host.ID())

	keys.SetContext(ctx)

	wg.Wait()
}
