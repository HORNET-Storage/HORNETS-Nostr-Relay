package sync

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/krpc"
	"github.com/anacrolix/torrent/bencode"
	"log"
	"reflect"
	"sort"
	"sync"
	"time"
)

type RelayStore struct {
	relays       map[string]NostrRelay
	mutex        sync.RWMutex
	dhtServer    *dht.Server
	uploadTicker *time.Ticker
	stopChan     chan struct{}
}

func NewRelayStore(dhtServer *dht.Server, uploadInterval time.Duration) *RelayStore {
	rs := &RelayStore{
		relays:       make(map[string]NostrRelay),
		dhtServer:    dhtServer,
		uploadTicker: time.NewTicker(uploadInterval),
		stopChan:     make(chan struct{}),
	}
	go rs.periodicUpload()
	return rs
}

func (rs *RelayStore) AddRelay(relay NostrRelay) {
	rs.mutex.Lock()
	defer rs.mutex.Unlock()
	rs.relays[relay.URL] = relay
}

func (rs *RelayStore) GetRelays() []NostrRelay {
	rs.mutex.RLock()
	defer rs.mutex.RUnlock()
	relays := make([]NostrRelay, 0, len(rs.relays))
	for _, relay := range rs.relays {
		relays = append(relays, relay)
	}
	return relays
}

func (rs *RelayStore) periodicUpload() {
	for {
		select {
		case <-rs.uploadTicker.C:
			rs.uploadToDHT()
		case <-rs.stopChan:
			rs.uploadTicker.Stop()
			return
		}
	}
}

func (rs *RelayStore) uploadToDHT() error {
	relays := rs.GetRelays()
	data, err := json.Marshal(relays)
	if err != nil {
		log.Println("Could not marshal relays:", err)
		return err
	}

	// Create a target for the DHT (you might want to use a more sophisticated key)
	target := CreateTarget([]byte("nostr:relay:%d")) // TODO: search for empty slot

	// Get a random DHT node to query
	addr, err := GetRandomDHTNode(rs.dhtServer)
	if err != nil {
		log.Println("Could not find random DHT node:", err)
		return err
	}

	seq := int64(1)
	// Get token for put operation
	token, err := GetDHTToken(rs.dhtServer, addr, seq, target)
	if err != nil {
		log.Println("Could not get dht token:", err)
		return err
	}

	// Create the bep44.Put structure
	// TODO: use the relay's key and store it
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		log.Println("Could not generate ed25519 keys:", err)
		return err
	}

	// Convert public key to the required [32]byte format
	var pubKey [32]byte
	copy(pubKey[:], publicKey)

	put := bep44.Put{
		V:    data,
		K:    &pubKey,
		Salt: []byte("nostr:relay"),
		Sig:  [64]byte{},
		Cas:  0,                     // Set to 0 if you're not using Compare-And-Swap
		Seq:  time.Now().UnixNano(), // Use current timestamp as sequence number
	}

	// Sign the put
	err = SignPut(&put, privateKey)
	if err != nil {
		log.Println("Could not sign put:", err)
		return err
	}

	// Perform the Put operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	putResult := rs.dhtServer.Put(ctx, addr, put, token, dht.QueryRateLimiting{})
	if putResult.Err != nil {
		return err
	}

	log.Println("Successfully uploaded relays to DHT")
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

//func GetDHTToken(dhtServer *dht.Server, addr dht.Addr, target krpc.ID) (*string, error) {
//	// First, we need to get a token for the Put operation
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//
//	getResult := dhtServer.Get(ctx, addr, target, nil, dht.QueryRateLimiting{})
//	if getResult.Err != nil {
//		return nil, getResult.Err
//	}
//
//	token := getResult.Reply.R.Token
//	if token == nil {
//		err := "error: No token received from DHT"
//		log.Println(err)
//		return nil, errors.New(err)
//	}
//
//	return token, nil
//}

func GetDHTToken(dhtServer *dht.Server, addr dht.Addr, seq int64, target krpc.ID) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Prepare the QueryInput
	//input := dht.QueryInput{
	//	MsgArgs: krpc.MsgArgs{
	//		InfoHash: target,
	//	},
	//	RateLimiting: dht.QueryRateLimiting{},
	//	NumTries:     3, // You can adjust this value as needed
	//}

	// Perform a get_peers query
	res := dhtServer.Get(ctx, addr, target, &seq, dht.QueryRateLimiting{})

	if res.ToError() != nil {
		return "", fmt.Errorf("DHT query failed: %w", res.ToError())
	}

	if res.Reply.R == nil {
		return "", fmt.Errorf("no response received from DHT node")
	}

	if res.Reply.R.Token == nil {
		return "", fmt.Errorf("no token in DHT response")
	}

	return *res.Reply.R.Token, nil
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

// Helper function to create a target (as discussed in previous messages)
func CreateTarget(value []byte) krpc.ID {
	//hash := sha1.Sum([]byte(key))
	//var target krpc.ID
	//copy(target[:], hash[:])

	//if i.IsMutable() {
	//	return sha1.Sum(append(i.K[:], i.Salt...))
	//}

	return sha1.Sum(bencode.MustMarshal(value))
}

func MarshalRelay(nr NostrRelay) ([]byte, error) {
	// Create a map to hold our data
	m := make(map[string]interface{})

	// Use reflection to get all fields
	v := reflect.ValueOf(nr)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i).Interface()
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			m[jsonTag] = value
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
