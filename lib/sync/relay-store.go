package sync

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/nbd-wtf/go-nostr"
	"io"
	"log"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

type Uploadable struct {
	payload   []byte
	pubkey    []byte
	signature [64]byte
}

type RelayStore struct {
	relays       map[string]ws.NIP11RelayInfo
	syncAuthors  map[string]bool
	syncTicker   *time.Ticker
	selfRelay    ws.NIP11RelayInfo
	libp2pHost   host.Host
	eventStore   *stores_graviton.GravitonStore
	mutex        sync.RWMutex
	dhtServer    *dht.Server
	uploadTicker *time.Ticker
	uploadables  *[]Uploadable
	stopChan     chan struct{}
}

type KeyPair struct {
	PrivKey []byte
	PubKey  []byte
}

// used for testing and keyless relay search (experimental)
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

const MaxRelays = 4

var (
	store      *RelayStore
	storeMutex sync.RWMutex
)

func NewRelayStore(dhtServer *dht.Server, host host.Host, eventStore *stores_graviton.GravitonStore, uploadInterval time.Duration, syncInterval time.Duration, self *ws.NIP11RelayInfo) *RelayStore {
	rs := &RelayStore{
		relays:       make(map[string]ws.NIP11RelayInfo),
		selfRelay:    *self,
		syncAuthors:  make(map[string]bool),
		libp2pHost:   host,
		eventStore:   eventStore,
		dhtServer:    dhtServer,
		uploadTicker: time.NewTicker(uploadInterval),
		syncTicker:   time.NewTicker(syncInterval),
		stopChan:     make(chan struct{}),
	}

	storeMutex.Lock()
	store = rs
	storeMutex.Unlock()

	go rs.periodicUpload()
	go rs.periodicSync()

	return rs
}

func GetRelayStore() *RelayStore {
	storeMutex.RLock()
	defer storeMutex.RUnlock()
	return store
}

func (rs *RelayStore) AddRelay(relay *ws.NIP11RelayInfo) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	// this dedupes
	rs.relays[relay.Pubkey] = *relay
}

func (rs *RelayStore) AddAuthor(authorPubkey string) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	// this also dedupes
	rs.syncAuthors[authorPubkey] = true
}

func (rs *RelayStore) GetRelays() []ws.NIP11RelayInfo {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	relays := make([]ws.NIP11RelayInfo, 0, len(rs.relays))
	for _, relay := range rs.relays {
		relays = append(relays, relay)
	}
	return relays
}

func (rs *RelayStore) GetSelfRelay() ws.NIP11RelayInfo {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	return rs.selfRelay
}

func (rs *RelayStore) AddUploadable(payload string, pubkey string, signature string, uploadNow bool) *Uploadable {
	payloadBytes, err := hex.DecodeString(payload)
	if err != nil {
		return nil
	}
	pubkeyBytes, err := hex.DecodeString(pubkey)
	if err != nil {
		return nil
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return nil
	}

	rs.mutex.Lock()
	uploadable := Uploadable{
		payload:   payloadBytes,
		pubkey:    pubkeyBytes,
		signature: [64]byte(sigBytes),
	}

	*rs.uploadables = append(*rs.uploadables, uploadable)
	if uploadNow {
		target, err := rs.doPutDelegated(uploadable)
		if err != nil {
			log.Printf("Error uploading %v: %v", uploadable.payload, err)
		} else {
			log.Printf("Successfully uploaded %v to %x", uploadable.payload, target)
		}
	}
	rs.mutex.Unlock()

	return &uploadable
}

func (rs *RelayStore) periodicSync() {
	for {
		select {
		case <-rs.syncTicker.C:
			for _, relay := range rs.relays {
				authors := []string{}
				for author := range rs.syncAuthors {
					authors = append(authors, author)
				}
				filter := nostr.Filter{Authors: authors}
				rs.SyncWithRelay(&relay, filter)
			}
		case <-rs.stopChan:
			rs.syncTicker.Stop()
			return
		}
	}
}

func (rs *RelayStore) periodicUpload() {
	for {
		select {
		case <-rs.uploadTicker.C:
			for _, uploadable := range *rs.uploadables {
				target, err := rs.doPutDelegated(uploadable)
				if err != nil {
					log.Printf("Error uploading %v: %v", uploadable.payload, err)
				} else {
					log.Printf("Successfully uploaded %v to %x", uploadable.payload, target)
				}
			}
		case <-rs.stopChan:
			rs.uploadTicker.Stop()
			return
		}
	}
}

func (rs *RelayStore) GetRelayListDHT(dhtKey *string) ([]*ws.NIP11RelayInfo, error) {
	keyBytes, err := hex.DecodeString(*dhtKey)
	if err != nil {
		return nil, err
	}
	emptySalt := []byte{}
	target := createMutableTarget(keyBytes, emptySalt)
	data, err := DoGet(rs.dhtServer, target, emptySalt)
	if err != nil {
		return nil, err
	}
	urls := ParseURLs(data)
	relays := []*ws.NIP11RelayInfo{}
	for _, url := range urls {
		relay := PerformNIP11Request(url)
		relays = append(relays, relay)
	}
	return relays, nil
}

func (rs *RelayStore) SyncWithRelay(relay *ws.NIP11RelayInfo, filter nostr.Filter) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if relay.HornetExtension == nil {
		log.Printf("Cannot sync with non-hornet relays, skipping sync")
		return
	}

	addrs := []ma.Multiaddr{}
	for _, addr := range relay.HornetExtension.LibP2PAddrs {
		multiAddr, err := ma.NewMultiaddr(addr)
		if err == nil {
			addrs = append(addrs, multiAddr)
		} else {
			log.Printf("Error creating multiaddr from %v: %v", addr, err)
		}
	}

	target := peer.AddrInfo{ID: peer.ID(relay.HornetExtension.LibP2PID), Addrs: addrs}
	if err := rs.libp2pHost.Connect(ctx, target); err != nil {
		log.Printf("Error connecting to %+v: %v", target, err)
	}

	// Open a stream to the peer
	stream, err := rs.libp2pHost.NewStream(ctx, target.ID, NegentropyProtocol)
	if err != nil {
		log.Printf("Error creating stream to %+v: %v", target, err)
	}

	err = InitiateEventSync(stream, filter, target.ID.String(), rs.eventStore)
	if err != nil {
		log.Printf("Error syncing events with %+v: %v", target, err)
	}

	err = stream.Close()
	if err != nil {
		log.Printf("Failed to close stream: %v", err)
		return
	}
}

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
			target := createMutableTarget(HardcodedKey.PubKey, salt)
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

func (rs *RelayStore) uploadToDHTSlot(freeSlot int) error {
	// Create a target for the DHT (you might want to use a more sophisticated key)
	salt := []byte(fmt.Sprintf("nostr:relay:%d", freeSlot))

	selfRelay := rs.GetSelfRelay()
	relayBytes, err := MarshalRelay(selfRelay)
	if err != nil {
		return err
	}

	target, err := DoPut(rs.dhtServer, relayBytes, salt, (*ed25519.PublicKey)(&HardcodedKey.PubKey), (*ed25519.PrivateKey)(&HardcodedKey.PrivKey))
	if err != nil {
		return err
	}

	log.Printf("Successfully uploaded self relay to DHT at target %x", target)
	return nil
}

func (rs *RelayStore) Stop() {
	close(rs.stopChan)
}

func SignPut(put *bep44.Put, privKey ed25519.PrivateKey) error {
	if len(privKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key size: expected %d, got %d", ed25519.PrivateKeySize, len(privKey))
	}

	signatureInput, err := createSignatureInput(put)
	if err != nil {
		return err
	}

	signature := ed25519.Sign(privKey, signatureInput)
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature size: expected %d, got %d", ed25519.SignatureSize, len(signature))
	}

	if len(put.Sig) != len(signature) {
		return fmt.Errorf("put.Sig field has incorrect size: expected %d, got %d", len(signature), len(put.Sig))
	}

	copy(put.Sig[:], signature)
	return nil
}

// Helper function to create the input for signing
func createSignatureInput(put *bep44.Put) ([]byte, error) {
	var buf bytes.Buffer

	if len(put.Salt) > 0 {
		buf.WriteString(fmt.Sprintf("4:salt%d:", len(put.Salt)))
		buf.Write(put.Salt)
	}

	buf.WriteString(fmt.Sprintf("3:seqi%d", put.Seq))
	// Bencode already prefixes the length of V before writing it
	buf.WriteString(fmt.Sprintf("e1:v"))

	// Encode and write the value
	encoder := bencode.NewEncoder(&buf)
	err := encoder.Encode(put.V)
	if err != nil {
		return nil, fmt.Errorf("failed to encode value: %w", err)
	}

	log.Println(buf.String())
	return buf.Bytes(), nil
}

func createTarget(value []byte) krpc.ID {
	return sha1.Sum(bencode.MustMarshal(value))
}

func createMutableTarget(pubKey []byte, salt []byte) krpc.ID {
	return sha1.Sum(append(pubKey[:], salt...))
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

func (rs *RelayStore) doPutDelegated(uploadable Uploadable) (krpc.ID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var target krpc.ID
	emptySalt := []byte{}
	target = createMutableTarget(uploadable.pubkey, emptySalt)
	log.Printf("Derived mutable target %x from %x", target, uploadable.pubkey)

	stats, err := getput.Put(ctx, target, rs.dhtServer, emptySalt, func(seq int64) bep44.Put {
		put := bep44.Put{
			V:   uploadable.payload,
			Seq: seq,
			Sig: uploadable.signature,
		}

		log.Printf("Put created %+v", put)

		return put
	})

	log.Printf("Put stats %v", stats)

	if err != nil {
		log.Printf("Put operation failed: %v", err)
		return target, errors.New("put operation failed")
	} else {
		log.Printf("Put operation successful")
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		log.Printf("Put operation timed out")
		return target, errors.New("put operation timed out")
	}

	return target, nil

}

func DoPut(server *dht.Server, value []byte, salt []byte, pubKey *ed25519.PublicKey, privKey *ed25519.PrivateKey) (krpc.ID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var target krpc.ID
	if privKey == nil {
		target = createTarget(value)
		log.Printf("Derived immutable target %x from %x", target, value)
	} else {
		target = createMutableTarget(*pubKey, salt)
		log.Printf("Derived mutable target %x from %x and %x", target, pubKey, salt)
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
			err := SignPut(&put, *privKey)
			if err != nil {
				log.Printf("Unable to sign")
			}
		}

		log.Printf("Put created %+v", put)

		return put
	})

	log.Printf("Put stats %v", stats)

	if err != nil {
		log.Printf("Put operation failed: %v", err)
	} else {
		log.Printf("Put operation successful")
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		log.Printf("Put operation timed out")
		return target, errors.New("Put operation timed out")
	}

	return target, nil
}

func DoGet(server *dht.Server, target bep44.Target, salt []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, stats, err := getput.Get(ctx, target, server, nil, salt)
	log.Printf("Get stats: %+v", stats)

	if err != nil {
		log.Printf("Get operation failed: %v", err)
		return nil, err
	}
	log.Printf("Get operation successful: %+v", result)

	var decodedValue []byte
	err = bencode.Unmarshal(result.V, &decodedValue)
	if err != nil {
		log.Printf("failed to unmarshal result: %v", err)
		return nil, err
	}

	return decodedValue, nil
}

func ParseURLs(input []byte) []string {
	var urlStrings []string
	err := json.Unmarshal(input, &urlStrings)
	if err != nil {
		log.Println("Error parsing JSON:", err)
		return []string{}
	}

	// Create a slice to store valid URLs
	var urls []string

	// Validate each URL
	for _, urlString := range urlStrings {
		// Trim any whitespace
		urlString = strings.TrimSpace(urlString)
		urls = append(urls, urlString)
	}

	return urls
}

func PerformNIP11Request(url string) *ws.NIP11RelayInfo {
	httpURL := strings.Replace(url, "wss://", "https://", 1)

	// Create a new request
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		log.Printf("Error creating request: %v", err)
		return nil
	}

	// Set the required headers for NIP-11
	req.Header.Set("Accept", "application/nostr+json")

	// Create a client with a timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Perform the request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Error performing request: %v", err)
		return nil
	}
	defer resp.Body.Close()

	// Check if the status code is 200 OK
	if resp.StatusCode != http.StatusOK {
		log.Printf("Error performing request, status: %d", resp.StatusCode)
		return nil
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
		return nil
	}

	// Unmarshal the JSON into NIP11RelayInfo struct
	var relayInfo ws.NIP11RelayInfo
	err = json.Unmarshal(body, &relayInfo)
	if err != nil {
		log.Printf("Error unmarshaling relay info: %v", err)
		return nil
	}

	return &relayInfo
}
