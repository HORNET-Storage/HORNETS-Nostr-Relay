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
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
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
	"gorm.io/gorm"
)

// RelayStore struct encapsulates the connection to the dht and the db where we store relay info
// It also handles syncing with other relays
type RelayStore struct {
	db           *gorm.DB
	syncTicker   *time.Ticker
	libp2pHost   host.Host
	eventStore   stores.Store
	mutex        sync.RWMutex
	dhtServer    *dht.Server
	uploadTicker *time.Ticker
	stopChan     chan struct{}
}

var (
	store      *RelayStore
	storeMutex sync.RWMutex
)

func NewRelayStore(
	db *gorm.DB,
	dhtServer *dht.Server,
	host host.Host,
	eventStore stores.Store,
	uploadInterval time.Duration,
	syncInterval time.Duration,
) *RelayStore {
	rs := &RelayStore{
		db:           db,
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

// AddRelay adds a given relay to the relay db
func (rs *RelayStore) AddRelay(relay *ws.NIP11RelayInfo) {
	log.Printf("Adding relay to relay store: %+v", relay)
	err := PutSyncRelay(rs.db, relay.Pubkey, relay)
	if err != nil {
		log.Printf("Error adding relay to relay store: %v", err)
	}
}

// AddAuthor adds a pubkey to the relay db
func (rs *RelayStore) AddAuthor(authorPubkey string) {
	log.Printf("Adding author to relay store: %s", authorPubkey)
	err := PutSyncAuthor(rs.db, authorPubkey)
	if err != nil {
		log.Printf("Error adding relay to relay store: %v", err)
	}
}

// AddUploadable saves an DHTUploadable to the relay db
func (rs *RelayStore) AddUploadable(payload string, pubkey string, signature string, uploadNow bool) error {
	log.Printf("Adding uploadable to sync store -- payload %s pubkey %s signature %s uploading now: %v", payload, pubkey, signature, uploadNow)

	payloadBytes, err := hex.DecodeString(payload)
	if err != nil {
		return err
	}
	pubkeyBytes, err := hex.DecodeString(pubkey)
	if err != nil {
		return err
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return err
	}

	err = PutDHTUploadable(rs.db, payloadBytes, pubkeyBytes, sigBytes)
	if err != nil {
		return err
	}

	return nil
}

// periodicSync handles periodically syncing events with known relays
func (rs *RelayStore) periodicSync() {
	for {
		select {
		case <-rs.syncTicker.C:
			syncAuthors, err := GetSyncAuthors(rs.db)
			if err != nil {
				log.Printf("Error getting relay authors: %v", err)
				continue
			}
			var authorNpubs []string
			for _, author := range syncAuthors {
				authorNpubs = append(authorNpubs, author.PublicKey)
			}

			relays, err := GetSyncRelays(rs.db)
			if err != nil {
				log.Printf("Error getting relays: %v", err)
				continue
			}

			log.Printf("Attempting sync with %d relays for %d authors", len(relays), len(syncAuthors))
			for _, relay := range relays {
				var relayInfo ws.NIP11RelayInfo
				err := json.Unmarshal([]byte(relay.RelayInfo), &relayInfo)
				if err != nil {
					log.Printf("Error unmarshaling relay info: %v", err)
					continue
				}

				filter := nostr.Filter{Authors: authorNpubs}
				rs.SyncWithRelay(&relayInfo, filter)
			}
		case <-rs.stopChan:
			rs.syncTicker.Stop()
			return
		}
	}
}

// periodicUpload periodically uploads the uploadables to the DHT
func (rs *RelayStore) periodicUpload() {
	for {
		select {
		case <-rs.uploadTicker.C:
			uploadables, err := GetDHTUploadables(rs.db)
			if err != nil {
				log.Printf("Error getting uploadables from DHT: %v", err)
				continue
			}
			log.Printf("Uploading %d user relay lists to dht", len(uploadables))
			for _, uploadable := range uploadables {
				target, err := rs.DoPut(uploadable)
				if err != nil {
					log.Printf("Error uploading %v: %v", uploadable.Payload, err)
				} else {
					log.Printf("Successfully uploaded %v to dht at target: %x", uploadable.Payload, target)
				}
			}
		case <-rs.stopChan:
			rs.uploadTicker.Stop()
			return
		}
	}
}

// GetRelayListFromDHT takes a dhtKey and asks the DHT for any data at the corresponding target
// It expects this data to be in the form of relay URLs, which we can later sync to
func (rs *RelayStore) GetRelayListFromDHT(dhtKey *string) ([]*ws.NIP11RelayInfo, error) {
	keyBytes, err := hex.DecodeString(*dhtKey)
	if err != nil {
		return nil, err
	}
	emptySalt := []byte{}
	target := CreateMutableTarget(keyBytes, emptySalt)
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

// SyncWithRelay will attempt to do a negentropy event sync with a relay specified by NIP11RelayInfo
// if the specified relay is not a hornet relay, then we do not try to sync
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

// SignPut takes a bep44 put object and formats it and signs it, putting the signature in put.Sig
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

// CreateMutableTarget derives the target (dht-input) for a given pubKey and salt
func CreateMutableTarget(pubKey []byte, salt []byte) krpc.ID {
	return sha1.Sum(append(pubKey[:], salt...))
}

// DoPut takes a DHTUploadable and puts it on the DHT
func (rs *RelayStore) DoPut(uploadable DHTUploadable) (krpc.ID, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var target krpc.ID
	emptySalt := []byte{}
	target = CreateMutableTarget(uploadable.Pubkey, emptySalt)
	log.Printf("Derived mutable target %x from pubkey %x", target, uploadable.Pubkey)

	sigBytes := [64]byte{}
	copy(sigBytes[:], uploadable.Signature)

	stats, err := getput.Put(ctx, target, rs.dhtServer, emptySalt, func(seq int64) bep44.Put {
		put := bep44.Put{
			V:   uploadable.Payload,
			Seq: seq,
			Sig: sigBytes,
		}

		log.Printf("Put created %+v", put)

		return put
	})

	log.Printf("DHT put stats %+v", stats)

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

// DoGet retrieves a given target from the dht
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

// PerformNIP11Request attempts to get NIP11 info from a given url
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
		log.Printf("Error performing NIP11 request: %v", err)
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
