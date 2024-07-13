package test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/nbd-wtf/go-nostr"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind0"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1984"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind3"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30008"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30009"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30023"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind5"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind6"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind7"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind8"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9372"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9373"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9735"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/universal"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/libp2p"

	handlers "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"

	net "github.com/libp2p/go-libp2p/core/network"
	peer "github.com/libp2p/go-libp2p/core/peer"
	// 	negentropy "github.com/illuzen/go-negentropy"
)

const DiscoveryServiceTag = "mdns-discovery"
const ProtocolID = "/testing/1.0.0"

// GenerateRandomEvent generates a random Nostr event using go-nostr
func GenerateRandomEvent() *nostr.Event {
	priv, err := signing.GeneratePrivateKey()
	if err != nil {
		log.Fatal("No private key provided and unable to make one from scratch. Exiting.")
	}
	pub := priv.PubKey()
	serializedPriv := hex.EncodeToString(priv.Serialize())
	serializedPub, err := signing.SerializePublicKeyBech32(pub)
	if err != nil {
		log.Fatal("Unable to serialize public key. Exiting.")
	}
	event := nostr.Event{
		PubKey:    *serializedPub,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      getRandomKind(),
		Tags:      nostr.Tags{nostr.Tag{"tag1", "value1"}, nostr.Tag{"tag2", "value2"}},
		Content:   randomHexString(256),
	}
	event.ID = event.GetID()
	err = event.Sign(serializedPriv)
	if err != nil {
		log.Fatal("Unable to sign. Exiting.", err)
	}

	return &event
}

// randomHexString generates a random hex string of a given length
func randomHexString(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(bytes)
}

// randInt generates a random integer between min and max
func randInt(min, max int) int {
	return min + rand.Intn(max-min)
}

// selectRandomItems selects n random items from the given slice.
func selectRandomItems(arr []int, n int) []int {
	if n > len(arr) {
		n = len(arr)
	}

	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())

	// Create a slice to store the selected items
	selected := make([]int, n)

	// Create a map to keep track of selected indices
	selectedIndices := make(map[int]bool)

	for i := 0; i < n; i++ {
		for {
			// Generate a random index
			index := rand.Intn(len(arr))

			// Ensure the index is not already selected
			if !selectedIndices[index] {
				selectedIndices[index] = true
				selected[i] = arr[index]
				break
			}
		}
	}

	return selected
}

func setupStore() *stores_graviton.GravitonStore {
	store := &stores_graviton.GravitonStore{}
	store.InitStore()

	handlers.RegisterHandler("universal", universal.BuildUniversalHandler(store))
	handlers.RegisterHandler("kind/0", kind0.BuildKind0Handler(store))
	handlers.RegisterHandler("kind/1", kind1.BuildKind1Handler(store))
	handlers.RegisterHandler("kind/3", kind3.BuildKind3Handler(store))
	handlers.RegisterHandler("kind/5", kind5.BuildKind5Handler(store))
	handlers.RegisterHandler("kind/6", kind6.BuildKind6Handler(store))
	handlers.RegisterHandler("kind/7", kind7.BuildKind7Handler(store))
	handlers.RegisterHandler("kind/8", kind8.BuildKind8Handler(store))
	handlers.RegisterHandler("kind/1984", kind1984.BuildKind1984Handler(store))
	handlers.RegisterHandler("kind/9735", kind9735.BuildKind9735Handler(store))
	handlers.RegisterHandler("kind/9372", kind9372.BuildKind9372Handler(store))
	handlers.RegisterHandler("kind/9373", kind9373.BuildKind9373Handler(store))
	handlers.RegisterHandler("kind/30023", kind30023.BuildKind30023Handler(store))
	handlers.RegisterHandler("kind/10000", kind10000.BuildKind10000Handler(store))
	handlers.RegisterHandler("kind/30000", kind30000.BuildKind30000Handler(store))
	handlers.RegisterHandler("kind/30008", kind30008.BuildKind30008Handler(store))
	handlers.RegisterHandler("kind/30009", kind30009.BuildKind30009Handler(store))

	return store
}

func GenerateRandomEvents(numEvents int, store *stores_graviton.GravitonStore) error {
	log.Printf("Generating %d random events and storing in graviton\n", numEvents)
	for i := 0; i < numEvents; i++ {
		event := GenerateRandomEvent()

		err := store.StoreEvent(event)
		if err != nil {
			return err
		}
	}
	return nil
}

func getRandomFilter() nostr.Filter {
	return nostr.Filter{
		Kinds: []int{getRandomKind()},
	}
}

func getRandomKind() int {
	kinds := []int{1, 3, 5, 6, 7, 8, 1984, 9735, 9372, 9373, 30023, 10000, 30000, 30008, 30009, 36810}
	return selectRandomItems(kinds, 1)[0]
}

func TestEventGenerationStorageRetrieval(t *testing.T) {
	log.Println("Testing event storage and retrieval.")
	store := setupStore()
	numEvents := 1000

	err := GenerateRandomEvents(numEvents, store)
	if err != nil {
		t.Fatalf("Error generating events: %v", err)
	}

	filter := nostr.Filter{}
	events, err := store.QueryEvents(filter)

	if err != nil {
		t.Fatalf("Error querying events: %v", err)
	}

	for _, event := range events {
		chk, err := event.CheckSignature()
		if err != nil || chk == false {
			t.Fatalf("Unable to check signature. Exiting. %v", err)
		}
	}
	log.Println("All signatures valid.")
}

func TestHostConnections(t *testing.T) {
	log.Println("Testing host connections.")
	//store := setupStore()
	numHosts := 10

	hosts := []host.Host{}

	for i := 0; i < numHosts; i++ {
		host := libp2p.GetHost("")
		defer host.Close()
		if err := libp2p.SetupMDNS(host, DiscoveryServiceTag); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Host %s addresses:\n", host.ID())
		for _, addr := range host.Addrs() {
			fmt.Printf("%s/p2p/%s\n", addr, host.ID())
		}
		hosts = append(hosts, host)
	}

	time.Sleep(2 * time.Second)

	for _, host := range hosts {
		if len(host.Network().Peers()) != numHosts-1 {
			t.Fatalf("Host %s has %d peers, expected %d", host.ID(), host.Peerstore().Peers().Len(), numHosts-1)
		}
	}
}

func TestHostCommunication(t *testing.T) {
	log.Println("Testing host syncing.")
	ctx := context.Background()

	store1 := setupStore()
	err := GenerateRandomEvents(100, store1)
	if err != nil {
		t.Fatalf("Error generating events: %v", err)
	}

	// 	store2 := setupStore()
	host1 := libp2p.GetHost("")
	host2 := libp2p.GetHost("")

	if err := libp2p.SetupMDNS(host1, DiscoveryServiceTag); err != nil {
		t.Fatal(err)
	}
	if err := libp2p.SetupMDNS(host2, DiscoveryServiceTag); err != nil {
		t.Fatal(err)
	}

	if err := host1.Connect(ctx, peer.AddrInfo{ID: host2.ID(), Addrs: host2.Addrs()}); err != nil {
		t.Fatal(err)
	}

	filter := getRandomFilter()
	events, err := store1.QueryEvents(filter)
	outgoing, err := json.Marshal(events)

	// Set a stream handler on the host
	host2.SetStreamHandler(ProtocolID, func(s net.Stream) {

		incoming, err := io.ReadAll(s)
		if err != nil {
			log.Println("Failed to read data from stream:", err)
			s.Reset()
			t.Fatal(err)
		}

		if bytes.Equal(incoming, outgoing) {
			fmt.Println("Data matches")
		} else {
			fmt.Printf("Received: %d bytes\n", len(incoming))
			fmt.Printf("Sent: %d bytes\n", len(outgoing))
			t.Fatal("Data mismatch")
		}

		s.Close()
	})

	// Open a stream to the peer
	s, err := host1.NewStream(ctx, host2.ID(), ProtocolID)
	if err != nil {
		t.Fatal(err)
	}

	// Send over connection
	_, err = s.Write(outgoing)
	if err != nil {
		s.Reset()
		t.Fatal(err)
	}
	//     fmt.Printf("Sent: %s\n", data)

	time.Sleep(2 * time.Second)

}
