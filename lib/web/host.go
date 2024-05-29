package web

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p/core/crypto"
	"log"
	"github.com/libp2p/go-libp2p"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"

	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

func GetHost(priv string) host.Host {
	key := priv
	if priv == "" {
		privGen, err := signing.GeneratePrivateKey()
		if err != nil {
			log.Fatal("No private key provided and unable to make one from scratch. Exiting.")
		}
		serializedPriv, err := signing.SerializePrivateKey(privGen)
		if err != nil {
			log.Fatal("Unable to serialize private key. Exiting.")
		}

		pub := privGen.PubKey()
		serializedPub, err := signing.SerializePublicKeyBech32(pub)
		if err != nil {
			log.Fatal("Unable to serialize public key. Exiting.")
		}

		log.Println("Generated public/private key pair: ", *serializedPub, "/", *serializedPriv)
		log.Println("Please copy the private key into your config.json file if you want to re-use it")

		key = *serializedPriv
	}

	decodedKey, err := signing.DecodeKey(key)
	if err != nil {
		log.Fatal(err)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(decodedKey)
	if err != nil {
		log.Fatal(err)
	}

	// Libp2p Host (0 => random port)
	listenAddress := "/ip4/127.0.0.1/udp/0/quic-v1"
	log.Printf("Starting server on %s\n", listenAddress)

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
		log.Fatal("Error starting server: #{err}")
	}

	fmt.Printf("Host started with id: %s\n", host.ID())

	return host
}


// Notifee receives notifications from mDNS service
type Notifee struct {
	h host.Host
}

// HandlePeerFound is called when new peer is found
func (n *Notifee) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("Host %s found peer: %s\n", n.h.ID(), pi.ID.Pretty())

	// Create a context with a timeout for the connection
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if err := n.h.Connect(ctx, pi); err != nil {
		fmt.Printf("Failed to connect to peer: %s\n", err)
	} else {
		fmt.Printf("Host %s connected to peer: %s\n", n.h.ID(), pi.ID.Pretty())
	}
}

func SetupMDNS(h host.Host, serviceTag string) error {
	n := &Notifee{h: h}
	mdnsService := mdns.NewMdnsService(h, serviceTag, n)
	return mdnsService.Start()
}