package middleware

import (
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

type PeerInfo struct {
	ID        peer.ID
	IP        string
	PublicKey crypto.PubKey
}

// Examples on how to extract data from libp2p stream but not needed right now
func SessionMiddleware(h host.Host) func(network.StreamHandler) network.StreamHandler {
	return func(next network.StreamHandler) network.StreamHandler {
		return func(stream network.Stream) {
			// Get remote peer
			/*
				remotePeer := stream.Conn().RemotePeer()

				/*
					protocols, err := h.Peerstore().GetProtocols(remotePeer)
					if err != nil {
						protocols = []protocol.ID{}
					}


				remoteAddr := stream.Conn().RemoteMultiaddr()
				ip, err := manet.ToIP(remoteAddr)
				if err != nil {
					return
				}

				// Extract information
				peerInfo := PeerInfo{
					ID: remotePeer,
					//Addrs:     h.Peerstore().Addrs(remotePeer),
					//Protocols: protocols,
					PublicKey: h.Peerstore().PubKey(remotePeer),
					IP:        ip.String(),
				}

				// Get latency (if available)
				//peerInfo.Latency = h.Peerstore().LatencyEWMA(remotePeer)

				json, err := json.Marshal(peerInfo)
				if err == nil {
					fmt.Println(string(json))
				}
			*/

			// Call the next handler
			next(stream)
		}
	}
}
