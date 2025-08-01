package test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	sync "github.com/HORNET-Storage/hornet-storage/lib/sync"
	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

func TestPutGetDHT(t *testing.T) {
	config := dht.NewDefaultServerConfig()
	server, err := dht.NewServer(config)
	require.NoError(t, err)
	defer server.Close()

	config2 := dht.NewDefaultServerConfig()
	server2, err := dht.NewServer(config2)
	require.NoError(t, err)
	defer server2.Close()

	// 1. Bootstrap the DHT
	//ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	//defer cancel()

	t.Log("Starting DHT bootstrap")
	_, err = server.Bootstrap()
	require.NoError(t, err)

	//Wait for nodes to be added to the routing table
	for i := 0; i < 30; i++ {
		stats := server.Stats()
		t.Logf("DHT stats: %+v", stats)
		if stats.GoodNodes > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// 2. Create a sample relay
	randomInt := rand.Intn(100000)
	sampleRelay := ws.NIP11RelayInfo{
		Name: fmt.Sprintf("Test Relay: %d", randomInt),
		HornetExtension: &ws.HornetExtension{
			LibP2PID:    "wss://example.com",
			LibP2PAddrs: []string{fmt.Sprintf("127.0.0.1:%d", randomInt)},
		},
	}
	relayBytes, err := sync.MarshalRelay(sampleRelay)
	require.NoError(t, err)

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	salt := []byte(fmt.Sprintf("nostr:relay:%d", randomInt))

	target, err := doPut(server, relayBytes, salt, &pubKey, &privKey)
	require.NoError(t, err)

	// Wait a bit for the value to propagate
	time.Sleep(5 * time.Second)

	// get it from the other server
	decodedValue, err := sync.DoGet(server2, target, salt)
	require.NoError(t, err)
	t.Logf("Got result: %+v", decodedValue)

	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, decodedValue, "", "  ")
	require.NoError(t, err)
	t.Logf("Received JSON:\n%s", prettyJSON.String())

	// 8. Verify the result
	foundRelay := ws.NIP11RelayInfo{}
	err = json.Unmarshal(decodedValue, &foundRelay)
	require.NoError(t, err)

	if sync.Equals(&sampleRelay, &foundRelay) == false {
		t.Fatalf("Sample and found relays do not match %v", err)
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

func TestPutGetLocal(t *testing.T) {
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

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	target, err := doPut(putServer, value, []byte{}, &pubKey, &privKey)
	require.NoError(t, err)

	// Wait for value to propagate
	time.Sleep(10 * time.Second)

	// Try to get from all servers
	var retrieved bool
	for i, getServer := range servers {
		t.Logf("Trying to get value from server %d with ID %x", i, getServer.ID())
		decodedValue, err := sync.DoGet(getServer, target, []byte{})
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

func TestPutAndSearchDHT(t *testing.T) {
	config := dht.NewDefaultServerConfig()
	server, err := dht.NewServer(config)
	require.NoError(t, err)
	defer server.Close()

	t.Log("Starting DHT bootstrap")
	_, err = server.Bootstrap()
	require.NoError(t, err)

	//Wait for nodes to be added to the routing table
	for i := 0; i < 30; i++ {
		stats := server.Stats()
		t.Logf("DHT stats: %+v", stats)
		if stats.GoodNodes > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// 2. Create a sample relay
	maxRelays := 3000
	randomInt := rand.Intn(maxRelays)
	// Generate a new private key
	nostrPriv, err := btcec.NewPrivateKey()
	if err != nil {
		panic(err)
	}

	// Get the corresponding public key
	nostrPub := nostrPriv.PubKey()
	sampleRelay := ws.NIP11RelayInfo{
		Pubkey:        hex.EncodeToString(nostrPub.SerializeCompressed()),
		SupportedNIPs: []int{1, 2, 3, 4, 5, 6, 7, 8, 9},
		HornetExtension: &ws.HornetExtension{
			LibP2PID:    "wss://example.com",
			LibP2PAddrs: []string{fmt.Sprintf("127.0.0.1:%d", randomInt)},
		},
	}
	err = ws.SignRelay(&sampleRelay, nostrPriv)
	require.NoError(t, err)
	relayBytes, err := sync.MarshalRelay(sampleRelay)
	require.NoError(t, err)

	saltStr := fmt.Sprintf("nostr:relay:%d", randomInt)
	salt := []byte(saltStr)
	t.Logf("salt: %s", saltStr)
	pubKey := ed25519.PublicKey(sync.HardcodedKey.PubKey)
	privKey := ed25519.PrivateKey(sync.HardcodedKey.PrivKey)

	target, err := doPut(server, relayBytes, salt, &pubKey, &privKey)
	require.NoError(t, err)
	t.Logf("put target %d: %x salt: %x", randomInt, target, salt)

	// Wait a bit for the value to propagate
	time.Sleep(5 * time.Second)

	relays, unoccupied := sync.SearchForRelays(server, maxRelays, randomInt-2, randomInt+2)

	t.Logf("found %d unoccupied slots: %v and %d relays: %v", len(unoccupied), unoccupied, len(relays), relays)

	require.True(t, len(relays) == 1)
	require.True(t, sync.Equals(&sampleRelay, &relays[0]))

}

func doPut(server *dht.Server, value []byte, salt []byte, pubKey *ed25519.PublicKey, privKey *ed25519.PrivateKey) (krpc.ID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var target krpc.ID
	if privKey == nil {
		target = createTarget(value)
		logging.Infof("Derived immutable target %x from %x", target, value)
	} else {
		target = sync.CreateMutableTarget(*pubKey, salt)
		logging.Infof("Derived mutable target %x from pubkey %x and salt %x", target, pubKey, salt)
	}

	stats, err := getput.Put(ctx, target, server, salt, func(seq int64) bep44.Put {
		put := bep44.Put{
			V:    value,
			Salt: salt,
			Seq:  seq,
		}

		if privKey != nil {
			var pub [32]byte
			copy(pub[:], *pubKey)
			put.K = &pub
			err := sync.SignPut(&put, *privKey)
			if err != nil {
				logging.Info("Unable to sign")
			}
		}

		logging.Infof("Put created %+v", put)

		return put
	})

	logging.Infof("DHT put stats %+v", stats)

	if err != nil {
		logging.Infof("Put operation failed: %v", err)
	} else {
		logging.Info("Put operation successful")
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		logging.Info("Put operation timed out")
		return target, errors.New("Put operation timed out")
	}

	return target, nil
}

// createTarget derives the target (dht-input) for a given byte array
// NOTE: this is only used in tests, since it is not the way targets are derived in bep44
func createTarget(value []byte) krpc.ID {
	return sha1.Sum(bencode.MustMarshal(value))
}
