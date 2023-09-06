package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"sync"
	"time"

	hornet_badger "github.com/HORNET-Storage/hornet-storage/lib/database/badger"
	"github.com/HORNET-Storage/hornet-storage/lib/web"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/transport"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"

	keys "github.com/HORNET-Storage/hornet-storage/lib/context"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers"
)

func main() {
	ctx := context.Background()

	wg := new(sync.WaitGroup)

	webFlag := flag.Bool("web", false, "Launch web server: true/false")
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

	//ctx = InitDatabase(ctx)
	//ctx = InitGrpcServer(ctx)

	// Database
	leafDatabase, err := hornet_badger.Open("leaves")
	if err != nil {
		log.Fatal(err)
	}

	contentDatabase, err := hornet_badger.Open("content")
	if err != nil {
		log.Fatal(err)
	}

	ctx = context.WithValue(ctx, keys.BlockDatabase, leafDatabase)
	ctx = context.WithValue(ctx, keys.ContentDatabase, contentDatabase)

	defer leafDatabase.Db.Close()
	defer contentDatabase.Db.Close()

	// Just a pre-generated key for the minute for testing purposes
	// TODO: Make this a launch param
	bytes, err := hex.DecodeString("0801124080c150e2d76a6832045b0e3766860b007da392870f736804caa695891ea19f0c89b604f23c7fe06941627a960326dbd28b422e751c65e86afef310fe6379a08e")
	if err != nil {
		panic(err)
	}

	priv, err := crypto.UnmarshalPrivateKey(bytes)
	if err != nil {
		panic(err)
	}

	/*
		priv, _, err := crypto.GenerateKeyPair(
			crypto.Ed25519, // Select your key type. Ed25519 are nice short
			-1,             // Select key length when possible (i.e. RSA).
		)

		if err != nil {
			panic(err)
		}
	*/

	//var idht *dht.IpfsDHT

	connmgr, err := connmgr.NewConnManager(
		100, // Lowwater
		400, // HighWater,
		connmgr.WithGracePeriod(time.Minute),
	)
	if err != nil {
		panic(err)
	}

	/*
		transport, err := libp2pquic.NewTransport(priv, nil, nil, nil, nil)
		if err != nil {
			panic(err)
		}

		serverAddress := "/ip4/0.0.0.0/udp/9000/quic" // replace this with the server's multiaddress
		maddr, err := multiaddr.NewMultiaddr(serverAddress)

		listener, err := transport.Listen(maddr)
		if err != nil {
			panic(err)
		}
	*/

	// Libp2p Host
	host, err := libp2p.New(
		libp2p.Identity(priv),
		// Multiple listen addresses
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/9000",
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

	host.SetStreamHandler("/upload/1.0.0", handlers.UploadStreamHandler)
	host.SetStreamHandler("/download/1.0.0", handlers.DownloadStreamHandler)
	host.SetStreamHandler("/branch/1.0.0", handlers.BranchStreamhandler)

	defer host.Close()

	ctx = context.WithValue(ctx, keys.Host, host)

	fmt.Printf("Host started with id: %s\n", host.ID())

	keys.SetContext(ctx)

	wg.Wait()
}

func InitDatabase(ctx context.Context) context.Context {

	return ctx
}

func customQUICConstructor(h host.Host, u *transport.Upgrader) (transport.Transport, error) {
	priv, _, err := crypto.GenerateKeyPair(
		crypto.Ed25519, // Select your key type. Ed25519 are nice short
		-1,             // Select key length when possible (i.e. RSA).
	)
	if err != nil {
		panic(err)
	}

	reuse, err := quicreuse.NewConnManager([32]byte{})
	if err != nil {
		panic(err)
	}

	return libp2pquic.NewTransport(priv, reuse, nil, nil, nil)
}

func privKeyToHex(privKey crypto.PrivKey) (string, error) {
	keyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(keyBytes), nil
}
