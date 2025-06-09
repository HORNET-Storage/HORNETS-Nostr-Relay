package main

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/auth"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind11011"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19841"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19842"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19843"
	"github.com/HORNET-Storage/hornet-storage/lib/moderation"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
	negentropy "github.com/HORNET-Storage/hornet-storage/lib/sync"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/fsnotify/fsnotify"
	"github.com/ipfs/go-cid"
	"github.com/spf13/viper"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/libp2p"

	"github.com/HORNET-Storage/hornet-storage/lib/web"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/count"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/filter"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind0"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10001"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10002"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10010"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1063"
	kind117handler "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind117"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind16629"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1984"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind3"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30008"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30009"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30023"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30078"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30079"
	kind411creator "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind411"
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

	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	//negentropy "github.com/illuzen/go-negentropy"
)

func init() {
	viper.SetDefault("private_key", "")
	viper.SetDefault("web", true)
	viper.SetDefault("proxy", true)
	viper.SetDefault("port", "9000")
	viper.SetDefault("demo_mode", false)
	viper.SetDefault("full_Text_kinds", []int{1})
	viper.SetDefault("relay_stats_db", "relay_stats.db")
	viper.SetDefault("query_cache", map[string]string{})
	viper.SetDefault("service_tag", "hornet-storage-service")
	viper.SetDefault("RelayName", "HORNETS")
	viper.SetDefault("RelayDescription", "The best relay ever.")
	viper.SetDefault("RelayPubkey", "")
	viper.SetDefault("RelaySupportedNips", []int{1, 11, 2, 9, 18, 23, 24, 25, 51, 56, 57, 42, 45, 50, 65, 116, 888, 555})
	viper.SetDefault("RelayContact", "support@hornets.net")
	viper.SetDefault("RelaySoftware", "golang")
	viper.SetDefault("RelayVersion", "0.0.1")
	viper.SetDefault("RelayDHTkey", "")
	viper.SetDefault("wallet_name", "")

	// Set default relay settings (including Mode)
	viper.SetDefault("relay_settings", map[string]interface{}{
		"Mode":             "whitelist", // Default mode to "whitelist"
		"IsKindsActive":    false,       // Default to false for activity flags
		"IsPhotosActive":   false,
		"IsVideosActive":   false,
		"IsGitNestrActive": false,
		"IsAudioActive":    false,
		"Kinds":            []string{}, // Default empty arrays for list fields
		"DynamicKinds":     []string{},
		"Photos":           []string{},
		"Videos":           []string{},
		"GitNestr":         []string{},
		"Audio":            []string{},
		"Protocol":         []string{}, // Default empty Protocol and Chunked lists
		"Chunked":          []string{},
		"KindWhitelist":    []string{"kind0", "kind1", "kind117", "kind22242", "kind10010", "kind19841", "kind19842", "kind19843", "kind1063"}, // Essential kinds always enabled
		"FreeTierEnabled":  true,
		"FreeTierLimit":    "100 MB per month",
		"ModerationMode":   "strict", // Default moderation mode to "strict"
		"subscription_tiers": []map[string]interface{}{
			{
				"DataLimit": "1 GB per month",
				"Price":     "1000", // in sats
			},
			{
				"DataLimit": "5 GB per month",
				"Price":     "10000", // in sats
			},
			{
				"DataLimit": "10 GB per month",
				"Price":     "15000", // in sats
			},
		},
	})

	// Generate a random wallet API key
	apiKey, err := generateRandomAPIKey()
	if err != nil {
		log.Fatalf("Failed to generate wallet API key: %v", err)
	}
	viper.SetDefault("wallet_api_key", apiKey)

	// Free tier settings are only used from relay_settings now
	viper.SetDefault("freeTierEnabled", true)
	viper.SetDefault("freeTierLimit", "100 MB per month")

	// Content filtering settings (direct Ollama integration)
	viper.SetDefault("ollama_url", "http://localhost:11434/api/generate")
	viper.SetDefault("ollama_model", "gemma3:1b")
	viper.SetDefault("ollama_timeout", 10000)
	viper.SetDefault("content_filter_cache_size", 10000)
	viper.SetDefault("content_filter_cache_ttl", 60)
	viper.SetDefault("content_filter_enabled", true)

	// Image moderation settings
	viper.SetDefault("image_moderation_enabled", true)
	viper.SetDefault("image_moderation_api", "http://localhost:8000/api/moderate")
	viper.SetDefault("image_moderation_threshold", 0.4)
	viper.SetDefault("image_moderation_mode", "full")
	viper.SetDefault("image_moderation_temp_dir", "/tmp/hornets-moderation")
	viper.SetDefault("image_moderation_check_interval", 30) // seconds
	viper.SetDefault("image_moderation_timeout", 60)        // seconds
	viper.SetDefault("image_moderation_concurrency", 5)

	// X-Nostr verification settings
	viper.SetDefault("xnostr_enabled", true)
	viper.SetDefault("xnostr_temp_dir", "/tmp/xnostr-verification")
	viper.SetDefault("xnostr_browser_path", "/usr/bin/chromium") // Default browser path
	viper.SetDefault("xnostr_browser_pool_size", 3)              // Default browser pool size
	viper.SetDefault("xnostr_update_interval", 24)               // hours
	viper.SetDefault("xnostr_check_interval", 30)                // seconds
	viper.SetDefault("xnostr_concurrency", 3)                    // concurrent verifications

	// X-Nostr verification intervals
	viper.SetDefault("xnostr_verification_intervals", map[string]interface{}{
		"full_verification_interval_days": 30,
		"follower_update_interval_days":   7,
		"max_verification_attempts":       5,
	})

	// X-Nostr Nitter settings
	viper.SetDefault("xnostr_nitter", map[string]interface{}{
		"instances": []map[string]interface{}{
			{"url": "https://nitter.net/", "priority": 1},
			{"url": "https://nitter.lacontrevoie.fr/", "priority": 2},
			{"url": "https://nitter.1d4.us/", "priority": 3},
			{"url": "https://nitter.kavin.rocks/", "priority": 4},
			{"url": "https://nitter.unixfox.eu/", "priority": 5},
			{"url": "https://nitter.fdn.fr/", "priority": 6},
			{"url": "https://nitter.pussthecat.org/", "priority": 7},
			{"url": "https://nitter.nixnet.services/", "priority": 8},
		},
		"requests_per_minute": 10,
		"failure_threshold":   3,
		"recovery_threshold":  2,
	})

	// We no longer need to set the top-level moderation_mode as we're using the one in relay_settings

	viper.AddConfigPath(".")
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}

	// Always force demo mode to false for the production server
	// This ensures authentication is enabled regardless of config.json settings
	viper.Set("demo_mode", false)

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

func generateDHTKey(privateKeyHex string) (string, error) {
	// Convert hex string to bytes
	privateKeyBytes, err := hex.DecodeString(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode private key hex: %v", err)
	}

	// Ensure we have the correct length
	if len(privateKeyBytes) != 32 {
		return "", fmt.Errorf("invalid private key length: expected 32 bytes, got %d", len(privateKeyBytes))
	}

	// Create a copy for clamping
	clampedPrivateKey := make([]byte, len(privateKeyBytes))
	copy(clampedPrivateKey, privateKeyBytes)

	// Apply clamping as per Ed25519 specification
	clampedPrivateKey[0] &= 248  // Clear the lowest 3 bits
	clampedPrivateKey[31] &= 127 // Clear the highest bit
	clampedPrivateKey[31] |= 64  // Set the second highest bit

	// Calculate hash using SHA-512
	hash := sha512.Sum512(clampedPrivateKey[:32])

	// In Ed25519, the first 32 bytes of the hash are used as the scalar
	// and the public key is derived using this scalar
	scalar := hash[:32]

	// For DHT key, we'll use the hex encoding of the scalar
	// This matches the behavior of the TypeScript implementation
	dhtKey := hex.EncodeToString(scalar)

	return dhtKey, nil
}

func main() {
	ctx := context.Background()
	wg := new(sync.WaitGroup)

	serializedPrivateKey := viper.GetString("private_key")

	// Generate a new private key and save it to viper config if one doesn't exist
	if serializedPrivateKey == "" {
		newKey, err := signing.GeneratePrivateKey()
		if err != nil {
			log.Printf("error generating or saving server private key")
		}

		serializedPrivateKey, err := signing.SerializePrivateKey(newKey)
		if err != nil {
			log.Printf("error generating or saving server private key")
		}

		viper.Set("private_key", serializedPrivateKey)
		err = viper.WriteConfig()
		if err != nil {
			log.Println("Viper has failed to write the config")
		}
	}

	if serializedPrivateKey != "" {
		// Generate DHT key from private key
		dhtKey, err := generateDHTKey(serializedPrivateKey)
		if err != nil {
			log.Printf("Failed to generate DHT key: %v", err)
		} else {
			err = viper.ReadInConfig()
			if err != nil {
				log.Println("Error reading viper config: ", err)
			}
			viper.Set("RelayDHTkey", dhtKey)
			err = viper.WriteConfig()
			if err != nil {
				log.Println("Error reading viper config: ", err)
			}
			log.Println("DHT key: ", dhtKey)

		}
	}

	privateKey, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey)
	if err != nil {
		log.Printf("failed to deserialize private key")
	}

	serializedPublicKey, err := signing.SerializePublicKey(publicKey)
	if err != nil {
		log.Printf("failed to serialize public key")
	}

	viper.Set("RelayPubkey", serializedPublicKey)

	host := libp2p.GetHostOnPort(serializedPrivateKey, viper.GetString("port"))

	// Create and initialize database
	store, err := badgerhold.InitStore("data")
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		err := store.Cleanup()
		if err != nil {
			log.Printf("Failed to cleanup temp database: %v", err)
		}

		// Shutdown moderation system if initialized
		moderation.Shutdown()
	}()

	// Initialize image moderation system if enabled
	if viper.GetBool("image_moderation_enabled") {
		log.Println("Initializing image moderation system...")

		// Get moderation configuration from viper
		apiEndpoint := viper.GetString("image_moderation_api")
		threshold := viper.GetFloat64("image_moderation_threshold")
		mode := viper.GetString("image_moderation_mode")
		timeout := time.Duration(viper.GetInt("image_moderation_timeout")) * time.Second
		checkInterval := time.Duration(viper.GetInt("image_moderation_check_interval")) * time.Second
		tempDir := viper.GetString("image_moderation_temp_dir")
		concurrency := viper.GetInt("image_moderation_concurrency")

		// Make sure temp directory exists
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			if err := os.MkdirAll(tempDir, 0755); err != nil {
				log.Printf("Failed to create moderation temp directory: %v", err)
				tempDir = os.TempDir() // Fallback to system temp dir
			}
		}

		// Initialize moderation system
		err := moderation.InitModeration(
			store,
			apiEndpoint,
			moderation.WithThreshold(threshold),
			moderation.WithMode(mode),
			moderation.WithTimeout(timeout),
			moderation.WithCheckInterval(checkInterval),
			moderation.WithTempDir(tempDir),
			moderation.WithConcurrency(concurrency),
		)

		if err != nil {
			log.Printf("Failed to initialize image moderation: %v", err)
		} else {
			log.Println("Image moderation system initialized successfully")
		}
	} else {
		log.Println("Image moderation system is disabled")
	}

	// Initialize the global subscription manager
	log.Println("Initializing global subscription manager...")
	var relaySettings lib.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &relaySettings); err != nil {
		log.Printf("Failed to load relay settings: %v", err)
	} else {
		subscription.InitGlobalManager(
			store,
			privateKey,
			viper.GetString("RelayDHTkey"),
			relaySettings.SubscriptionTiers,
		)
		log.Println("Global subscription manager initialized successfully")
	}

	// Create and store kind 411 event
	if err := kind411creator.CreateKind411Event(privateKey, publicKey, store); err != nil {
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

	syncDB, err := negentropy.InitSyncDB()
	if err != nil {
		log.Fatal("failed to connect database: %w", err)
	}

	negentropy.SetupNegentropyEventHandler(host, "host", store)
	skipdht := true
	if !skipdht {
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

	}

	// X-Nostr verification is now handled in the frontend
	log.Println("X-Nostr verification is now handled in the frontend")

	// Register Our Nostr Stream Handlers
	if settings.Mode == "blacklist" {
		log.Println("Using universal stream handler because Mode set to 'blacklist'")
		nostr.RegisterHandler("universal", universal.BuildUniversalHandler(store))
	} else if settings.Mode == "whitelist" {
		log.Println("Using specific stream handlers because Mode set to 'whitelist'")
		nostr.RegisterHandler("kind/0", kind0.BuildKind0Handler(store, privateKey))
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
		nostr.RegisterHandler("kind/22242", auth.BuildAuthHandler(store))
		nostr.RegisterHandler("kind/30000", kind30000.BuildKind30000Handler(store))
		nostr.RegisterHandler("kind/30008", kind30008.BuildKind30008Handler(store))
		nostr.RegisterHandler("kind/30009", kind30009.BuildKind30009Handler(store))
		nostr.RegisterHandler("kind/30023", kind30023.BuildKind30023Handler(store))
		nostr.RegisterHandler("kind/30078", kind30078.BuildKind30078Handler(store))
		nostr.RegisterHandler("kind/30079", kind30079.BuildKind30079Handler(store))
		nostr.RegisterHandler("kind/16629", kind16629.BuildKind16629Handler(store))
		nostr.RegisterHandler("kind/10010", kind10010.BuildKind10010Handler(store))
		nostr.RegisterHandler("kind/19841", kind19841.BuildKind19841Handler(store))
		nostr.RegisterHandler("kind/19842", kind19842.BuildKind19842Handler(store))
		nostr.RegisterHandler("kind/19843", kind19843.BuildKind19843Handler(store))
		nostr.RegisterHandler("kind/117", kind117handler.BuildKind117Handler(store))
		nostr.RegisterHandler("kind/1063", kind1063.BuildKind1063Handler(store))

		// X-Nostr verification is now handled in the frontend
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
			err = web.StartServer(store)

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
			app := ws.BuildServer(store)

			//app.Get("/scionic/upload", fiber_websocket.New(upload.AddUploadHandlerForWebsockets(store, canUpload, handleUpload)))

			err := ws.StartServer(app)

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

		store.Cleanup()

		os.Exit(0)
	}()

	wg.Wait()
}
