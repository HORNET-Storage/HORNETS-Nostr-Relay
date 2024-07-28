package test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	sync "github.com/HORNET-Storage/hornet-storage/lib/sync"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDHTPutGet(t *testing.T) {
	config := dht.NewDefaultServerConfig()
	config.StartingNodes = func() ([]dht.Addr, error) {
		addrs, err := dht.GlobalBootstrapAddrs("")
		if err != nil {
			t.Fatalf("Unable to get bootstrap addrs %v", err)
			return nil, err
		}
		//log.Println("addrs", addrs)
		return addrs, nil
	}

	server, err := dht.NewServer(config)
	require.NoError(t, err)
	defer server.Close()

	// 1. Bootstrap the DHT
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Log("Starting DHT bootstrap")
	_, err = server.Bootstrap()
	require.NoError(t, err)
	//t.Log("Stats:", stats)

	// Wait for nodes to be added to the routing table
	for i := 0; i < 30; i++ {
		stats := server.Stats()
		t.Logf("DHT stats: %+v", stats)
		if stats.GoodNodes > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// 2. Create a sample relay
	sampleRelay := sync.NostrRelay{
		URL:  "wss://example.com",
		Name: "Test Relay",
	}
	relayBytes, err := json.Marshal(sampleRelay)
	require.NoError(t, err)

	publicKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Unable to generate ed25519 key %v", err)
	}

	addr, err := sync.GetRandomDHTNode(server)
	require.NoError(t, err)

	// Convert public key to the required [32]byte format
	var pubKey [32]byte
	copy(pubKey[:], publicKey)

	// 3. Create a BEP44 Put
	put := bep44.Put{
		V:    relayBytes,
		K:    &pubKey,
		Salt: []byte("nostr:relay"),
		Sig:  [64]byte{},
		Cas:  0,                     // Set to 0 if you're not using Compare-And-Swap
		Seq:  time.Now().UnixNano(), // Use current timestamp as sequence number
	}

	// 4. Sign the put
	err = sync.SignPut(&put, privKey)
	require.NoError(t, err)
	t.Log("Signed put: ", put.Sig)

	target := sync.CreateTarget("nostr:relay:1")
	token, err := sync.GetDHTToken(server, addr, target)
	require.NoError(t, err)

	// 5. Announce the relay to the DHT
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	putResult := server.Put(ctx, addr, put, *token, dht.QueryRateLimiting{})
	require.NoError(t, putResult.ToError())

	// 6. Wait a bit for the value to propagate
	time.Sleep(5 * time.Second)

	// 7. Search for the relay
	addr2, err := sync.GetRandomDHTNode(server)
	require.NoError(t, err)
	result := server.Get(ctx, addr2, target, nil, dht.QueryRateLimiting{})
	require.NoError(t, err)
	require.NotNil(t, result)

	// 8. Verify the result
	foundRelay := sync.NostrRelay{}
	err = json.Unmarshal(result.Reply.R.V, &foundRelay)
	require.NoError(t, err)

	if sampleRelay.URL != foundRelay.URL {
		t.Fatalf("Sample and found relay urls do not match %v", err)
	}
	if sampleRelay.Name != foundRelay.Name {
		t.Fatalf("Sample and found relay names do not match %v", err)
	}
}

//func setupLocalDHTNetwork(t *testing.T, nodeCount int) []*dht.Server {
//	servers := make([]*dht.Server, nodeCount)
//	for i := 0; i < nodeCount; i++ {
//		config := dht.NewDefaultServerConfig()
//		config.StartingNodes = func() ([]dht.Addr, error) { return nil, nil }
//
//		s, err := dht.NewServer(config)
//		require.NoError(t, err)
//		servers[i] = s
//	}
//
//	// Connect the nodes to each other
//	for i, s := range servers {
//		for j, other := range servers {
//			if i != j {
//				err := s.AddNode(other.)
//				if err != nil {
//					return nil
//				}
//			}
//		}
//	}
//
//	return servers
//}
