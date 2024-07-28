package sync

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	"log"
	"math/rand"
	"net"
	"time"
)

type NostrRelay struct {
	URL           string `json:"url"`
	PublicKey     string `json:"public_key"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	SupportedNIPs []int  `json:"supported_nips"`
	Signature     []byte `json:"signature"`
}

// Pack Nostr relay array into DHT structure
func packNostrRelays(relays []NostrRelay) (*krpc.Return, error) {
	// Serialize the relay array to JSON
	data, err := json.Marshal(relays)
	if err != nil {
		return nil, err
	}

	// Create the Return structure
	ret := &krpc.Return{
		Bep44Return: krpc.Bep44Return{
			V: data,
		},
	}

	return ret, nil
}

// Unpack DHT structure into Nostr relay array
func unpackNostrRelays(ret *krpc.Return) ([]NostrRelay, error) {
	if ret == nil || ret.Bep44Return.V == nil {
		return nil, errors.New("no data in return value")
	}

	var relays []NostrRelay
	err := json.Unmarshal(ret.Bep44Return.V, &relays)
	if err != nil {
		return nil, err
	}

	return relays, nil
}

func GetRandomDHTNode(s *dht.Server) (dht.Addr, error) {
	// Get all nodes from the routing table
	nodes := s.Nodes()

	// Check if we have any nodes
	if len(nodes) == 0 {
		return nil, errors.New("no nodes in routing table")
	}

	// Generate a random index
	rand.Seed(time.Now().UnixNano())
	randomIndex := rand.Intn(len(nodes))

	// Get the randomly selected node
	selectedNode := nodes[randomIndex]

	// Convert krpc.NodeInfo to dht.Addr
	addr := dht.NewAddr(&net.UDPAddr{
		IP:   selectedNode.Addr.IP,
		Port: selectedNode.Addr.Port,
	})

	return addr, nil
}

func searchForRelays(d *dht.Server, maxRelays int) []NostrRelay {
	var relays []NostrRelay
	ch := make(chan NostrRelay, maxRelays)

	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start multiple goroutines to search in parallel
	for i := 0; i < maxRelays; i++ {
		go func(i int) {
			// Create a deterministic key
			key := fmt.Sprintf("nostr:relay:%d", i)
			hash := sha1.Sum([]byte(key))

			// Convert to bep44.Target
			var target bep44.Target
			copy(target[:], hash[:])

			addr, err := GetRandomDHTNode(d)
			if err != nil {
				return
			}

			// Set sequence number (optional)
			var seq *int64
			// If you want to use a sequence number:
			// seqValue := int64(1)
			// seq = &seqValue

			// Set rate limiting (you might need to adjust this based on your needs)
			rl := dht.QueryRateLimiting{}

			// Perform DHT get operation
			result := d.Get(ctx, addr, target, seq, rl)
			log.Print(result.Reply)

			relays, err := unpackNostrRelays(result.Reply.R)
			for _, relay := range relays {
				ch <- relay
			}
		}(i)
	}

	// Collect results
	for i := 0; i < maxRelays; i++ {
		select {
		case relay := <-ch:
			relays = append(relays, relay)
		case <-ctx.Done():
			return relays
		}
	}

	return relays
}
