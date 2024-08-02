package test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	sync "github.com/HORNET-Storage/hornet-storage/lib/sync"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
	"github.com/stretchr/testify/require"
	"math/rand"
	"net"
	"testing"
	"time"
)

func TestDHTPutGet(t *testing.T) {
	nodeCount := 5
	servers := setupLocalDHTNetwork(t, nodeCount)

	//config := dht.NewDefaultServerConfig()
	//server, err := dht.NewServer(config)
	//require.NoError(t, err)
	//defer server.Close()
	//
	//config2 := dht.NewDefaultServerConfig()
	//server2, err := dht.NewServer(config2)
	//require.NoError(t, err)
	//defer server2.Close()

	// 1. Bootstrap the DHT
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	//t.Log("Starting DHT bootstrap")
	//_, err = server.Bootstrap()
	//require.NoError(t, err)
	//t.Log("Stats:", stats)

	// Wait for nodes to be added to the routing table
	//for i := 0; i < 30; i++ {
	//	stats := server.Stats()
	//	t.Logf("DHT stats: %+v", stats)
	//	if stats.GoodNodes > 0 {
	//		break
	//	}
	//	time.Sleep(2 * time.Second)
	//}

	// 2. Create a sample relay
	sampleRelay := sync.NostrRelay{
		URL:  "wss://example.com",
		Name: "Test Relay",
	}
	relayBytes, err := sync.MarshalRelay(sampleRelay)
	require.NoError(t, err)

	publicKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Unable to generate ed25519 key %v", err)
	}

	// Convert public key to the required [32]byte format
	var pubKey [32]byte
	copy(pubKey[:], publicKey)

	seq := int64(1)
	// 3. Create a BEP44 Put
	put := bep44.Put{
		V: relayBytes,
		K: &pubKey,
		//Salt: []byte("nostr:relay"),
		Sig: [64]byte{},
		Cas: 0, // Set to 0 if you're not using Compare-And-Swap
		Seq: seq,
	}

	// 4. Sign the put
	err = sync.SignPut(&put, privKey)
	require.NoError(t, err)
	t.Log("Signed put: ", put.Sig)

	randomInt := rand.Intn(100000)
	targetInput := fmt.Sprintf("nostr:relay:%d", randomInt)

	target := sync.CreateTarget([]byte(targetInput))
	t.Log("Created target: ", target, " from input: ", targetInput)

	seqToPut := makeSeqToPut(t, true, true, put, privKey)
	stats, err := getput.Put(ctx, target, servers[0], []byte{}, seqToPut)
	if err != nil {
		t.Fatalf("Unable to put to network %v", err)
	}
	t.Logf("Got put stats: %v", stats)

	//var token string
	//var addr dht.Addr
	//var putResult dht.QueryResult
	//for i := 0; i < 5; i++ {
	//	addr, err = sync.GetRandomDHTNode(server)
	//	if err != nil {
	//		continue
	//	}
	//	t.Logf("Got random DHT Node: %s", addr)
	//
	//	token, err = sync.GetDHTToken(server, addr, seq, target)
	//	if err != nil {
	//		continue
	//	}
	//	t.Logf("Got token: %x", token)
	//
	//	// 5. Announce the relay to the DHT
	//	t.Logf("Putting %s to addr %s ", target, addr)
	//	putResult = server.Put(ctx, addr, put, token, dht.QueryRateLimiting{})
	//	if putResult.Reply.E != nil {
	//		t.Logf("Putting to addr %s failed: %s", addr, putResult.Reply.E)
	//		continue
	//	}
	//	t.Logf("Got put result: %+v", putResult)
	//	t.Logf("Result.Reply: %+v", putResult.Reply)
	//	t.Logf("Result.Reply.R: %+v", putResult.Reply.R)
	//	t.Logf("Raw V value: %s", string(putResult.Reply.R.V))
	//	t.Logf("Raw V value (hex): %x", putResult.Reply.R.V)
	//	break
	//}
	//require.NoError(t, err)
	//require.NotNil(t, token)
	//require.NoError(t, putResult.Reply.E)

	// 6. Wait a bit for the value to propagate
	time.Sleep(15 * time.Second)

	// 7. Search for the relay
	//t.Log("Getting another Random DHT Node")
	//addr2, err := sync.GetRandomDHTNode(server)
	//require.NoError(t, err)

	//t.Logf("Getting target %s from addr %s", target, addr)
	result, stats, err := getput.Get(ctx, target, servers[nodeCount-1], &seq, []byte{})
	t.Logf("Got get stats: %v", stats)
	//result := server.Get(ctx, addr, target, &seq, dht.QueryRateLimiting{})
	//result := server.Get(ctx, addr2, target, nil, dht.QueryRateLimiting{})
	require.NoError(t, err)
	require.NotNil(t, result)

	t.Logf("Got result: %+v", result)
	t.Logf("Got result.V: %+v", result.V)
	//t.Logf("Result.Reply: %+v", result.Reply)
	//t.Logf("Result.Reply.R: %+v", result.Reply.R)
	//t.Logf("Raw Bep44Return value: %s", string(result.Reply.R.Bep44Return.V))
	//t.Logf("Raw Bep44Return value (hex): %x", result.Reply.R.Bep44Return.V)

	//var getResult dht.QueryResult
	//if result.Reply.R.Bep44Return.V == nil {
	//	for _, node := range result.Reply.R.Nodes {
	//		t.Logf("Trying: %+v", node)
	//
	//		nodeAddr, err := net.ResolveUDPAddr("udp", node.String())
	//		if err != nil {
	//			continue
	//		}
	//
	//		// Convert krpc.NodeInfo to dht.Addr
	//		addr := dht.NewAddr(&net.UDPAddr{
	//			IP:   nodeAddr.IP,
	//			Port: nodeAddr.Port,
	//		})
	//
	//		getResult = server.Get(ctx, addr, target, &seq, dht.QueryRateLimiting{})
	//		if getResult.Err == nil && getResult.Reply.R.Bep44Return.V != nil {
	//			t.Logf("Got result: %+v", getResult)
	//			result = getResult
	//			break
	//		}
	//	}
	//}

	// 8. Verify the result
	foundRelay := sync.NostrRelay{}
	//err = json.Unmarshal(result.Reply.R.Bep44Return.V, &foundRelay)
	err = json.Unmarshal(result.V, &foundRelay)
	require.NoError(t, err)

	if sampleRelay.URL != foundRelay.URL {
		t.Fatalf("Sample and found relay urls do not match %v", err)
	}
	if sampleRelay.Name != foundRelay.Name {
		t.Fatalf("Sample and found relay names do not match %v", err)
	}
}

func setupLocalDHTNetwork(t *testing.T, nodeCount int) []*dht.Server {
	t.Logf("Creating %d dht servers", nodeCount)
	servers := make([]*dht.Server, nodeCount)
	for i := 0; i < nodeCount; i++ {
		config := dht.NewDefaultServerConfig()
		config.StartingNodes = func() ([]dht.Addr, error) { return nil, nil }
		config.NoSecurity = true // For testing purposes

		// Create a UDP connection bound to localhost with a random port
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
		require.NoError(t, err)
		config.Conn = conn

		// Set the public IP to localhost for local testing
		config.PublicIP = net.IPv4(127, 0, 0, 1)

		s, err := dht.NewServer(config)
		require.NoError(t, err)
		servers[i] = s
	}

	// Connect the nodes to each other
	t.Logf("Connecting %d dht servers to each other", nodeCount)
	for i, s := range servers {
		for j, other := range servers {
			if i != j {
				otherAddr := other.Addr().(*net.UDPAddr)
				nodeAddr := krpc.NodeAddr{
					IP:   otherAddr.IP,
					Port: otherAddr.Port,
				}
				nodeInfo := krpc.NodeInfo{
					ID:   other.ID(),
					Addr: nodeAddr,
				}
				err := s.AddNode(nodeInfo)
				require.NoError(t, err)
			}
		}
	}

	verifyConnections(t, servers)

	return servers
}

func makeSeqToPut(t *testing.T, autoSeq, mutable bool, put bep44.Put, privKey ed25519.PrivateKey) getput.SeqToPut {
	return func(seq int64) bep44.Put {
		// Increment best seen seq by one.
		if autoSeq {
			put.Seq = seq + 1
		}
		if mutable {
			err := sync.SignPut(&put, privKey)
			if err != nil {
				t.Fatalf("Could not sign put %v", err)
			}
		}
		return put
	}
}

func verifyConnections(t *testing.T, servers []*dht.Server) {
	// Give servers some time to recognize each other
	time.Sleep(5 * time.Second)
	t.Logf("Verifying connections for %d servers", len(servers))

	for i, server := range servers {

		// Get the routing table
		nodes := server.Nodes()
		//t.Logf("Server %d has %d nodes in its routing table", i, len(nodes))

		// Check if the server knows about all other servers
		for j, otherServer := range servers {
			if i == j {
				continue // Skip self
			}

			found := false
			for _, node := range nodes {
				if node.ID == otherServer.ID() {
					found = true
					//t.Logf("Server %d knows about server %d (ID: %x)", i, j, otherServer.ID())
					break
				}
			}

			if !found {
				t.Errorf("Server %d does not know about server %d (ID: %x)", i, j, otherServer.ID())
			}
		}

		// Ping test
		for j, otherServer := range servers {
			if i == j {
				continue // Skip self
			}

			result := server.Ping(otherServer.Addr().(*net.UDPAddr))
			if result.Err != nil {
				t.Logf("Server %d failed to ping server %d: %v", i, j, result.Err)
			} else {
				//t.Logf("Server %d successfully pinged server %d", i, j)
			}
		}
	}
}

func doPut(t *testing.T, server *dht.Server, key bep44.Target, value []byte, salt []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stats, err := getput.Put(ctx, key, server, salt, func(seq int64) bep44.Put {
		return bep44.Put{V: value, Salt: salt}
	})

	t.Logf("Put stats %v", stats)

	if err != nil {
		t.Fatalf("Put operation failed: %v", err)
	} else {
		t.Logf("Put operation successful")
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("Put operation timed out")
	}
}

func doGet(t *testing.T, server *dht.Server, key bep44.Target, salt []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, stats, err := getput.Get(ctx, key, server, nil, salt)
	t.Logf("Get stats: %+v", stats)
	t.Logf("Result: %+v", result)

	if err != nil {
		t.Logf("Get operation failed: %v", err)
		return nil, err
	}

	t.Logf("Get operation successful")
	return result.V, nil
}

func TestPutAndGetLocal(t *testing.T) {
	nodeCount := 5
	servers := setupLocalDHTNetwork(t, nodeCount)
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	putServer := servers[rand.Intn(nodeCount)]
	t.Logf("Using server with ID %x for put operation", putServer.ID())

	value := []byte("test value")
	target := sync.CreateTarget(value)

	doPut(t, putServer, target, value, []byte{})

	// Wait for value to propagate
	time.Sleep(10 * time.Second)

	// Try to get from all servers
	var retrieved bool
	for i, getServer := range servers {
		t.Logf("Trying to get value from server %d with ID %x", i, getServer.ID())
		retrievedValue, err := doGet(t, getServer, target, []byte{})
		require.NoError(t, err)
		var decodedValue []byte
		err = bencode.Unmarshal(retrievedValue, &decodedValue)
		require.NoError(t, err)

		if bytes.Equal(value, decodedValue) {
			t.Logf("Successfully retrieved correct value from server %d", i)
			retrieved = true
			break
		} else {
			t.Logf("Retrieved value doesn't match the original from server %d", i)
		}
	}

	require.True(t, retrieved, "Failed to retrieve the correct value from any server")
}
