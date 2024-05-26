package test

import (
	"encoding/hex"
	"log"
	"math/rand"
	"time"
	"testing"
	handlers "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	universalhandler "github.com/HORNET-Storage/hornet-storage/lib/handlers/universal"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind0"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1984"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind3"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30008"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30009"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30023"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind36810"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind5"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind6"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind7"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind8"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9372"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9373"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9735"
	"github.com/nbd-wtf/go-nostr"
)


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
	kinds := []int{1, 3, 5, 6, 7, 8, 1984, 9735, 9372, 9373, 30023, 10000, 30000, 30008, 30009, 36810}
	event := nostr.Event{
		PubKey:    *serializedPub,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      selectRandomItems(kinds, 1)[0],
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

func TestEventGenerationStorageRetrieval(t *testing.T) {
	store := &stores_graviton.GravitonStore{}
	store.InitStore()
	numEvents := 1000
	log.Println("Testing event storage and retrieval.")

	log.Printf("Generating %d random events and storing in graviton\n", numEvents)
	for i := 0; i < numEvents; i++ {
		event := GenerateRandomEvent()
		//log.Println(event)

		err := store.StoreEvent(event)
		if err != nil {
			t.Fatalf("Error storing event: %v", err)

		}
	}

	handlers.RegisterHandler("universal", universalhandler.BuildUniversalHandler(store))
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
	handlers.RegisterHandler("kind/36810", kind36810.BuildKind36810Handler(store))


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