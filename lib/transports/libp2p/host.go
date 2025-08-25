package libp2p

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/p2p/muxer/yamux"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	libp2pwebtransport "github.com/libp2p/go-libp2p/p2p/transport/webtransport"
	"github.com/multiformats/go-multiaddr"

	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

func generateKey() *string {
	privGen, err := signing.GeneratePrivateKey()
	if err != nil {
		logging.Fatal("No private key provided and unable to make one from scratch. Exiting.")
	}
	serializedPriv, err := signing.SerializePrivateKey(privGen)
	if err != nil {
		logging.Fatal("Unable to serialize private key. Exiting.")
	}

	pub := privGen.PubKey()
	serializedPub, err := signing.SerializePublicKeyBech32(pub)
	if err != nil {
		logging.Fatal("Unable to serialize public key. Exiting.")
	}

	// Use UpdateConfig with save=false for runtime-only values
	// These are libp2p keys, not the main relay keys, and shouldn't persist
	config.UpdateConfig("key", *serializedPub, false)
	config.UpdateConfig("priv_key", *serializedPriv, false)
	logging.Infof("Generated public/private key pair: %s/%s", *serializedPub, *serializedPriv)
	logging.Info("Please copy the private key into your config.json file if you want to re-use it")

	return serializedPriv
}

func GetHost(priv string) host.Host {
	key := priv
	if priv == "" {
		key = *generateKey()
	}

	decodedKey, err := signing.DecodeKey(key)
	if err != nil {
		logging.Fatalf("Failed to decode key: %v", err)
	}

	privateKey, err := crypto.UnmarshalSecp256k1PrivateKey(decodedKey)
	if err != nil {
		logging.Fatalf("Failed to unmarshal secp256k1 private key: %v", err)
	}

	// Libp2p Host (0 => random port)
	listenAddress := "/ip4/127.0.0.1/udp/0/quic-v1"
	webtransportListenAddress := "/ip4/127.0.0.1/udp/0/quic/webtransport"
	logging.Infof("Starting server on %s\n", listenAddress)

	host, err := libp2p.New(
		libp2p.Identity(privateKey),
		// Multiple listen addresses
		libp2p.ListenAddrStrings(
			listenAddress,
			webtransportListenAddress,
		),
		// support TLS connections
		//libp2p.Security(libp2ptls.ID, libp2ptls.New),
		// support noise connections
		//libp2p.Security(noise.ID, noise.New),

		//libp2p.Transport(customQUICConstructor),
		// support any other default transports (TCP)
		//libp2p.DefaultTransports,
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(libp2pwebtransport.New),
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
		logging.Fatalf("Error starting server: %v", err)
	}

	logging.Infof("Host started with id: %s\n", host.ID())

	return host
}

type connectionNotifier struct{}

func (n *connectionNotifier) Connected(net network.Network, conn network.Conn) {
	logging.Infof("Connected to: %s\n", conn.RemotePeer().String())
}

func (n *connectionNotifier) Disconnected(net network.Network, conn network.Conn) {
	logging.Infof("Disconnected from: %s\n", conn.RemotePeer().String())
}

func (n *connectionNotifier) Listen(net network.Network, multiaddr multiaddr.Multiaddr) {
	logging.Infof("Started listening on: %s\n", multiaddr)
}

func (n *connectionNotifier) ListenClose(net network.Network, multiaddr multiaddr.Multiaddr) {
	logging.Infof("Stopped listening on: %s\n", multiaddr)
}

func GetHostOnPort(serializedPrivateKey string, port string) host.Host {
	privateKey, _, err := signing.DeserializePrivateKey(serializedPrivateKey)
	if err != nil {
		logging.Fatalf("Failed to deserialize private key: %v", err)
	}

	libp2pPrivateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKey.Serialize())
	if err != nil {
		logging.Fatalf("Failed to unmarshal libp2p private key: %v", err)
	}

	listenAddress := fmt.Sprintf("/ip4/0.0.0.0/udp/%s/quic-v1", port)
	webtransportListenAddress := fmt.Sprintf("/ip4/0.0.0.0/udp/%s/quic/webtransport", port)
	logging.Infof("Starting server on %s\n", listenAddress)

	connManager, err := connmgr.NewConnManager(
		50,  // Low water mark
		100, // High water mark
		connmgr.WithGracePeriod(time.Second*30),
	)
	if err != nil {
		logging.Fatalf("Failed to create connection manager: %v", err)
	}

	host, err := libp2p.New(
		libp2p.Identity(libp2pPrivateKey),
		libp2p.ListenAddrStrings(listenAddress, webtransportListenAddress),
		libp2p.ConnectionManager(connManager),
		libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Security(noise.ID, noise.New),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Transport(libp2pwebtransport.New),
	)
	if err != nil {
		logging.Fatalf("Error starting server: %v", err)
	}

	notifier := &connectionNotifier{}
	host.Network().Notify(notifier)

	logging.Infof("Host started with id: %s\n", host.ID())

	return host
}

// Notifee receives notifications from mDNS service
type Notifee struct {
	h host.Host
}

// HandlePeerFound is called when new peer is found
func (n *Notifee) HandlePeerFound(pi peer.AddrInfo) {
	logging.Infof("Host %s found peer: %s\n", n.h.ID(), pi.ID)

	// Create a context with a timeout for the connection
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	if err := n.h.Connect(ctx, pi); err != nil {
		logging.Infof("Failed to connect to peer: %s\n", err)
	} else {
		logging.Infof("Host %s connected to peer: %s\n", n.h.ID(), pi.ID)
	}
}

func SetupMDNS(h host.Host, serviceTag string) error {
	n := &Notifee{h: h}
	mdnsService := mdns.NewMdnsService(h, serviceTag, n)
	return mdnsService.Start()
}
