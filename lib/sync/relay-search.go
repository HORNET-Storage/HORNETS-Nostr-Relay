package sync

// NOTE: this file is code for an experimental mechanism that searches the
// Mainline DHT for unknown relays at predictable targets using a hardcoded key and enumerated salts
import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"

	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/anacrolix/dht/v2"
)

type KeyPair struct {
	PrivKey []byte
	PubKey  []byte
}

// used to make targets predictable
var HardcodedKey = KeyPair{
	PrivKey: []byte{
		0x51, 0x8d, 0x31, 0x74, 0x5e, 0x17, 0x14, 0x28,
		0xf4, 0xbc, 0x5e, 0x2c, 0x88, 0xae, 0x2f, 0x36,
		0x37, 0x7a, 0xc2, 0xf4, 0xd3, 0xe1, 0x38, 0x68,
		0xac, 0xc6, 0x9f, 0x3f, 0x88, 0x99, 0x2b, 0xdb,
		0x6b, 0x9f, 0x74, 0x78, 0x36, 0x89, 0x4f, 0xc2,
		0xc6, 0xcd, 0xbe, 0x8d, 0xce, 0x52, 0xc1, 0xaf,
		0xc1, 0xc9, 0x48, 0xb5, 0x72, 0xf0, 0xc6, 0x62,
		0x3a, 0x07, 0xcf, 0x77, 0xb5, 0xb8, 0xf8, 0x7f,
	},
	PubKey: []byte{
		0x6b, 0x9f, 0x74, 0x78, 0x36, 0x89, 0x4f, 0xc2,
		0xc6, 0xcd, 0xbe, 0x8d, 0xce, 0x52, 0xc1, 0xaf,
		0xc1, 0xc9, 0x48, 0xb5, 0x72, 0xf0, 0xc6, 0x62,
		0x3a, 0x07, 0xcf, 0x77, 0xb5, 0xb8, 0xf8, 0x7f,
	},
}

func MarshalRelay(nr ws.NIP11RelayInfo) ([]byte, error) {
	m := make(map[string]interface{})

	v := reflect.ValueOf(nr)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i).Interface()
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			// Split the json tag to get just the field name
			parts := strings.Split(jsonTag, ",")
			key := parts[0]

			// Only add non-empty values if "omitempty" is specified
			if len(parts) > 1 && parts[1] == "omitempty" {
				if !reflect.ValueOf(value).IsZero() {
					m[key] = value
				}
			} else {
				m[key] = value
			}
		}
	}

	// Get sorted keys
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create a new map with sorted keys
	sorted := make(map[string]interface{})
	for _, k := range keys {
		sorted[k] = m[k]
	}

	// Marshal the sorted map
	return json.Marshal(sorted)
}

// SearchForRelays searches salt-space for relays to sync with
// The target is derived from a hardcoded key (because it needs to be predictable) and an
// enumeration of possible salts "nostr:relay:%d"
// NOTE: this is not currently used outside of tests
func SearchForRelays(d *dht.Server, maxRelays int, minIndex int, maxIndex int) ([]ws.NIP11RelayInfo, []int) {
	log.Printf("Searching for relays from %d to %d", minIndex, maxIndex)
	type result struct {
		index int
		relay ws.NIP11RelayInfo
		found bool
	}

	var relays []ws.NIP11RelayInfo
	var unoccupiedSlots []int
	ch := make(chan result, maxIndex-minIndex)

	// Create a context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start multiple goroutines to search in parallel
	for i := minIndex; i < maxIndex; i++ {
		go func(i int) {
			// Create salt
			salt := []byte(fmt.Sprintf("nostr:relay:%d", i))
			target := CreateMutableTarget(HardcodedKey.PubKey, salt)
			fmt.Printf("get target %d: %x salt: %x\n", i, target, salt)

			// Perform DHT get operation
			data, err := DoGet(d, target, salt)
			if err != nil {
				ch <- result{index: i, found: false}
				return
			}

			foundRelay := ws.NIP11RelayInfo{}
			err = json.Unmarshal(data, &foundRelay)
			if err != nil {
				fmt.Printf("Could not unmarshall into NostrRelay %x : %v\n", data, err)
				ch <- result{index: i, found: false}
				return
			}

			err = CheckSig(&foundRelay)
			if err != nil {
				fmt.Printf("Signature verification failed %+v : %v\n", foundRelay, err)
				ch <- result{index: i, found: false}
				return
			}

			ch <- result{index: i, relay: foundRelay, found: true}
		}(i)
	}

	// Collect results
	foundCount := 0
	for i := 0; i < maxIndex-minIndex; i++ {
		//fmt.Printf("waiting for %d\n", i)
		select {
		case res := <-ch:
			if res.found { // Check if a relay was found
				//fmt.Printf("found %d\n", i)
				relays = append(relays, res.relay)
				foundCount++
			} else {
				//fmt.Printf("not found %d\n", i)
				unoccupiedSlots = append(unoccupiedSlots, res.index)
			}
			if foundCount >= maxRelays && len(unoccupiedSlots) > 0 {
				return relays, unoccupiedSlots
			}
		case <-ctx.Done():
			return relays[:foundCount], unoccupiedSlots
		}
	}

	// Trim any unfilled slots
	return relays[:foundCount], unoccupiedSlots
}
