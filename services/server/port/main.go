package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind11011"
	negentropy "github.com/HORNET-Storage/hornet-storage/lib/sync"

	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
	"github.com/joho/godotenv"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/fsnotify/fsnotify"
	"github.com/ipfs/go-cid"
	"github.com/spf13/viper"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	fiber_websocket "github.com/gofiber/contrib/websocket"

	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/libp2p"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/HORNET-Storage/hornet-storage/lib/web"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/count"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/filter"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind0"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10001"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10002"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1984"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind3"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30008"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30009"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30023"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30079"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind5"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind6"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind7"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind8"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9372"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9373"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9735"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind9802"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/universal"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic/download"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic/query"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic/upload"

	//stores_memory "github.com/HORNET-Storage/hornet-storage/lib/stores/memory"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	//negentropy "github.com/illuzen/go-negentropy"
)

func init() {
	viper.SetDefault("key", "")
	viper.SetDefault("web", false)
	viper.SetDefault("proxy", true)
	viper.SetDefault("port", "9000")
	viper.SetDefault("relay_stats_db", "relay_stats.db")
	viper.SetDefault("query_cache", map[string]string{})
	viper.SetDefault("service_tag", "hornet-storage-service")
	viper.SetDefault("RelayName", "HORNETS")
	viper.SetDefault("RelayDescription", "The best relay ever.")
	viper.SetDefault("RelayPubkey", "")
	viper.SetDefault("RelayContact", "support@hornets.net")
	viper.SetDefault("RelaySoftware", "golang")
	viper.SetDefault("RelayVersion", "0.0.1")
	viper.SetDefault("RelayDHTkey", "")

	// Generate a random wallet API key
	apiKey, err := generateRandomAPIKey()
	if err != nil {
		log.Fatalf("Failed to generate wallet API key: %v", err)
	}
	viper.SetDefault("wallet_api_key", apiKey)

	viper.SetDefault("subscription_tiers", []map[string]interface{}{
		{
			"data_limit": "1 GB per month",
			"price":      10000, // in sats
		},
		{
			"data_limit": "5 GB per month",
			"price":      40000, // in sats
		},
		{
			"data_limit": "10 GB per month",
			"price":      70000, // in sats
		},
	})

	viper.AddConfigPath(".")
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		log.Println("Config file changed:", e.Name)
	})

	viper.WatchConfig()
}

// Helper function to generate a random 32-byte hexadecimal key
func generateRandomAPIKey() (string, error) {
	bytes := make([]byte, 32) // 32 bytes = 256 bits
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func loadOrCreateEnvFile(envFile string) {
	// Check if the .env file exists
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		// If the file doesn't exist, create it
		f, err := os.Create(envFile)
		if err != nil {
			log.Fatalf("Error creating .env file: %s", err)
		}
		defer f.Close()

		// Optionally, write default environment variables to the .env file
		_, err = f.WriteString("KEY=default_value\n")
		if err != nil {
			log.Fatalf("Error writing to .env file: %s", err)
		}

		fmt.Printf(".env file created at %s\n", envFile)
	}

	// Load the .env file
	err := godotenv.Load(envFile)
	if err != nil {
		log.Printf("Error loading .env file: %s", err)
		return
	}

	fmt.Printf(".env file loaded from %s\n", envFile)
}

func main() {

	envFile := ".env"
	loadOrCreateEnvFile(envFile)

	ctx := context.Background()

	wg := new(sync.WaitGroup)

	viper.Set("key", os.Getenv("NOSTR_PUBLIC_KEY"))

	// Private key
	key := viper.GetString("key")

	host := libp2p.GetHostOnPort(key, viper.GetString("port"))

	// Create and initialize database
	store := &stores_graviton.GravitonStore{}

	queryCache := viper.GetStringMapString("query_cache")
	err := store.InitStore("gravitondb", queryCache)
	if err != nil {
		log.Fatal(err)
	}

	// generate server priv key if it does not exist
	err = generateAndSaveNostrPrivateKey()
	if err != nil {
		log.Printf("error generating or saving server private key")
	}

	err = godotenv.Load(envFile)
	if err != nil {
		log.Printf("error loading .env file: %s", err)
		return
	}

	// load keys from environment for signing kind 411
	privKey, pubKey, err := loadSecp256k1Keys()
	if err != nil {
		log.Printf("error loading keys from environment. check if you have the key in the environment: %s", err)
		return
	}
	// Create dht key for using relay private key and set it on viper.
	_, _, err = generateEd25519Keypair(os.Getenv("NOSTR_PRIVATE_KEY"))
	if err != nil {
		log.Printf("error generating dht-key: %s", err)
		return
	}

	// Create and store kind 411 event
	if err := createKind411Event(privKey, pubKey, store); err != nil {
		log.Printf("Failed to create kind 411 event: %v", err)
		return
	}

	// generate server priv key if it does not exist
	err = generateAndSaveNostrPrivateKey()
	if err != nil {
		log.Printf("error generating or saving server private key")
	}
	// load keys from environment for signing kind 411
	privKey, pubKey, err = loadSecp256k1Keys()
	if err != nil {
		log.Printf("error loading keys from environment. check if you have the key in the environment: %s", err)
		return
	}
	// TODO: We need to only generate it once. When it does not exist.
	// Create dht key for using relay private key and set it on viper.
	_, _, err = generateEd25519Keypair(os.Getenv("NOSTR_PRIVATE_KEY"))
	if err != nil {
		log.Printf("error generating dht-key: %s", err)
		return
	}

	// Create and store kind 411 event
	if err := createKind411Event(privKey, pubKey, store); err != nil {
		log.Printf("Failed to create kind 411 event: %v", err)
		return
	}

	// Stream Handlers
	download.AddDownloadHandler(host, store, func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		return true
	})

	canUpload := func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		decodedSignature, err := hex.DecodeString(*signature)
		if err != nil {
			return false
		}

		parsedSignature, err := schnorr.ParseSignature(decodedSignature)
		if err != nil {
			return false
		}

		contentID, err := cid.Parse(rootLeaf.Hash)
		if err != nil {
			return false
		}

		publicKey, err := signing.DeserializePublicKey(*pubKey)
		if err != nil {
			return false
		}

		err = signing.VerifyCIDSignature(parsedSignature, contentID, publicKey)
		return err == nil
	}

	handleUpload := func(dag *merkle_dag.Dag, pubKey *string) {}

	upload.AddUploadHandlerForLibp2p(ctx, host, store, canUpload, handleUpload)

	query.AddQueryHandler(host, store)

	settings, err := nostr.LoadRelaySettings()
	if err != nil {
		log.Fatalf("Failed to load relay settings: %v", err)
		return
	}

	log.Printf("Host started with id: %s\n", host.ID())
	log.Printf("Host started with address: %s\n", host.Addrs())

	syncDB, err := negentropy.InitSyncDB("sync_store.db")
	if err != nil {
		log.Fatal("failed to connect database")
	}

	negentropy.SetupNegentropyEventHandler(host, "host", store)

	libp2pAddrs := []string{}
	for _, addr := range host.Addrs() {
		libp2pAddrs = append(libp2pAddrs, addr.String())
	}
	viper.Set("LibP2PID", host.ID().String())
	viper.Set("LibP2PAddrs", libp2pAddrs)
	selfRelay := ws.GetRelayInfo()
	log.Printf("Self Relay: %+v\n", selfRelay)

	dhtServer := negentropy.DefaultDHTServer()
	defer dhtServer.Close()

	// this periodically syncs with other relays, and uploads user keys to dht
	uploadInterval := time.Hour * 2
	syncInterval := time.Hour * 3
	relayStore := negentropy.NewRelayStore(syncDB, dhtServer, host, store, uploadInterval, syncInterval)
	log.Printf("Created relay store: %+v", relayStore)

	// Register Our Nostr Stream Handlers
	if settings.Mode == "unlimited" {
		log.Println("Using universal stream handler because Mode set to 'unlimited'")
		nostr.RegisterHandler("universal", universal.BuildUniversalHandler(store))
	} else if settings.Mode == "smart" {
		log.Println("Using specific stream handlers because Mode set to 'smart'")
		nostr.RegisterHandler("kind/0", kind0.BuildKind0Handler(store))
		nostr.RegisterHandler("kind/1", kind1.BuildKind1Handler(store))
		nostr.RegisterHandler("kind/3", kind3.BuildKind3Handler(store))
		nostr.RegisterHandler("kind/5", kind5.BuildKind5Handler(store))
		nostr.RegisterHandler("kind/6", kind6.BuildKind6Handler(store))
		nostr.RegisterHandler("kind/7", kind7.BuildKind7Handler(store))
		nostr.RegisterHandler("kind/8", kind8.BuildKind8Handler(store))
		nostr.RegisterHandler("kind/1984", kind1984.BuildKind1984Handler(store))
		nostr.RegisterHandler("kind/9735", kind9735.BuildKind9735Handler(store))
		nostr.RegisterHandler("kind/9372", kind9372.BuildKind9372Handler(store))
		nostr.RegisterHandler("kind/9373", kind9373.BuildKind9373Handler(store))
		nostr.RegisterHandler("kind/9802", kind9802.BuildKind9802Handler(store))
		nostr.RegisterHandler("kind/10000", kind10000.BuildKind10000Handler(store))
		nostr.RegisterHandler("kind/10001", kind10001.BuildKind10001Handler(store))
		nostr.RegisterHandler("kind/10002", kind10002.BuildKind10002Handler(store))
		nostr.RegisterHandler("kind/11011", kind11011.BuildKind11011Handler(store))
		nostr.RegisterHandler("kind/30000", kind30000.BuildKind30000Handler(store))
		nostr.RegisterHandler("kind/30008", kind30008.BuildKind30008Handler(store))
		nostr.RegisterHandler("kind/30009", kind30009.BuildKind30009Handler(store))
		nostr.RegisterHandler("kind/30023", kind30023.BuildKind30023Handler(store))
		nostr.RegisterHandler("kind/30079", kind30079.BuildKind30079Handler(store))
	} else {
		log.Fatalf("Unknown settings mode: %s, exiting", settings.Mode)
	}

	nostr.RegisterHandler("filter", filter.BuildFilterHandler(store))
	nostr.RegisterHandler("count", count.BuildCountsHandler(store))

	// Auth event not supported for the libp2p connections yet
	//nostr.RegisterHandler("auth", auth.BuildAuthHandler(store))

	// Register a libp2p handler for every stream handler
	for kind := range nostr.GetHandlers() {
		handler := nostr.GetHandler(kind)

		wrapper := func(stream network.Stream) {
			read := func() ([]byte, error) {
				decoder := json.NewDecoder(stream)

				var rawMessage json.RawMessage
				err := decoder.Decode(&rawMessage)
				if err != nil {
					return nil, err
				}

				return rawMessage, nil
			}

			write := func(messageType string, params ...interface{}) {
				response := nostr.BuildResponse(messageType, params)

				if len(response) > 0 {
					stream.Write(response)
				}
			}

			handler(read, write)

			stream.Close()
		}

		host.SetStreamHandler(protocol.ID("/nostr/event/"+kind), middleware.SessionMiddleware(host)(wrapper))
	}

	// Web Panel
	if viper.GetBool("web") {
		wg.Add(1)

		log.Println("Starting with web server enabled")

		go func() {
			err := web.StartServer()

			if err != nil {
				log.Println("Fatal error occurred in web server")
			}

			wg.Done()
		}()
	}

	// Proxy web sockets
	if viper.GetBool("proxy") {
		wg.Add(1)

		log.Println("Starting with legacy nostr proxy web server enabled")

		go func() {
			app := websocket.BuildServer(store)

			app.Get("/scionic/upload", fiber_websocket.New(upload.AddUploadHandlerForWebsockets(store, canUpload, handleUpload)))

			err := websocket.StartServer(app)

			if err != nil {
				log.Println("Fatal error occurred in web server")
			}

			wg.Done()
		}()
	}

	// Handle kill signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		os.Exit(0)
	}()

	wg.Wait()
}
