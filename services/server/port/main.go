package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"

	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"

	"github.com/HORNET-Storage/hornet-storage/lib/moderation"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"

	"github.com/HORNET-Storage/hornet-storage/lib/upnp"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/ipfs/go-cid"
	"github.com/spf13/viper"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/HORNET-Storage/hornet-storage/lib/sessions/libp2p/middleware"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/libp2p"

	"github.com/HORNET-Storage/hornet-storage/lib/web"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/auth"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/count"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/filter"

	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind0"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10001"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10002"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10010"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind10411"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1063"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind11011"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind117"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind16629"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind16630"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1808"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1984"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19841"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19842"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19843"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind3"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30000"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30008"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30009"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30023"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind30078"
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

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	negentropy "github.com/HORNET-Storage/hornet-storage/lib/sync"
	ws "github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
)

func init() {
	// Initialze config system
	err := config.InitConfig()
	if err != nil {
		logging.Fatalf("Failed to initialize config: %v", err)
	}

	// Initialize logging system
	if err := logging.InitLogger(); err != nil {
		logging.Fatalf("Failed to initialize logger: %v", err)
	}

	// Now use the logging system
	logging.Info("HORNETS Nostr Relay starting up", map[string]interface{}{
		"version": viper.GetString("relay.version"),
		"name":    viper.GetString("relay.name"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Initialze upnp system if enabled
	if viper.GetBool("server.upnp") {
		upnp, err := upnp.Init(ctx)
		if err != nil {
			logging.Error("UPnP init failed", map[string]interface{}{
				"error": err,
			})
		}

		ip, err := upnp.ExternalIP()
		if err == nil {
			logging.Info("UPnP External IP", map[string]interface{}{
				"ip": ip,
			})
		}
	}
}

func main() {
	ctx := context.Background()
	wg := new(sync.WaitGroup)

	settings, err := config.GetConfig()
	if err != nil {
		logging.Fatal("Failed to load configuration", map[string]interface{}{
			"error": err,
		})
	}

	serializedPrivateKey := viper.GetString("relay.private_key")

	if len(serializedPrivateKey) <= 0 {
		newKey, err := signing.GeneratePrivateKey()
		if err != nil {
			logging.Fatal("error generating or saving server private key")
		}

		key, err := signing.SerializePrivateKey(newKey)
		if err != nil {
			logging.Fatal("error generating or saving server private key")
		} else {
			viper.Set("relay.private_key", key)
			serializedPrivateKey = *key

			// Also derive and save the public key
			_, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey)
			if err != nil {
				logging.Fatal("error deriving public key from generated private key")
			}

			serializedPublicKey, err := signing.SerializePublicKey(publicKey)
			if err != nil {
				logging.Fatal("error serializing public key")
			} else {
				viper.Set("relay.public_key", serializedPublicKey)
			}

			err = config.SaveConfig()
			if err != nil {
				logging.Fatal("Failed to save configuration", map[string]interface{}{
					"error": err,
				})
			}

			logging.Info("Generated new server keys", map[string]interface{}{
				"private_key": serializedPrivateKey,
				"public_key":  serializedPublicKey,
			})
		}
	}

	dhtKey := viper.GetString("relay.dht_key")

	if len(dhtKey) <= 0 {
		dhtKey, err := negentropy.DeriveKeyFromNsec(serializedPrivateKey)
		if err != nil {
			logging.Errorf("Failed to generate DHT key: %v", err)
		} else {
			viper.Set("relay.dht_key", dhtKey)

			err = config.SaveConfig()
			if err != nil {
				logging.Fatal("Failed to save configuration", map[string]interface{}{
					"error": err,
				})
			}

			logging.Info("Generated new server DHT key", map[string]interface{}{
				"dht_key": dhtKey,
			})
		}
	}

	// Generate wallet API key if not set
	walletAPIKey := viper.GetString("external_services.wallet.key")

	if len(walletAPIKey) <= 0 {
		newAPIKey, err := config.GenerateRandomAPIKey()
		if err != nil {
			logging.Errorf("Failed to generate wallet API key: %v", err)
		} else {
			viper.Set("external_services.wallet.key", newAPIKey)

			err = config.SaveConfig()
			if err != nil {
				logging.Fatal("Failed to save configuration", map[string]interface{}{
					"error": err,
				})
			}

			logging.Info("Generated new wallet API key", map[string]interface{}{
				"wallet_api_key": newAPIKey,
			})
		}
	}

	privateKey, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey)
	if err != nil {
		logging.Fatal("failed to deserialize private key")
	}

	// Ensure public key is always derived from private_key on relay start no matter what the public key is in the config
	serializedPublicKey, err := signing.SerializePublicKey(publicKey)
	if err != nil {
		logging.Errorf("Failed to serialize public key: %v", err)
	} else {
		viper.Set("relay.public_key", *serializedPublicKey)

		err = config.SaveConfig()
		if err != nil {
			logging.Errorf("Failed to save public key to config: %v", err)
		} else {
			logging.Info("Saved public key to configuration", map[string]interface{}{
				"public_key": *serializedPublicKey,
			})
		}
	}

	port := config.GetPort("hornets")
	portStr := fmt.Sprintf("%d", port)

	host := libp2p.GetHostOnPort(serializedPrivateKey, portStr)

	if viper.GetBool("server.upnp") {
		upnp := upnp.Get()

		err = upnp.ForwardPort(uint16(port), "LaterCondition")
		if err != nil {
			logging.Error("Failed to forward port using UPnP", map[string]interface{}{
				"port": port,
			})
		}
	}

	// Create and initialize database
	store, err := badgerhold.InitStore(config.GetPath("store"))
	if err != nil {
		logging.Fatal(err.Error())
	}

	// Initialize image moderation system if enabled
	if config.IsEnabled("content_filtering.image_moderation.enabled") {
		defer func() {
			err := store.Cleanup()
			if err != nil {
				logging.Infof("Failed to cleanup temp database: %v", err)
			}

			// Shutdown moderation system if initialized
			moderation.Shutdown()
		}()

		logging.Info("Initializing image moderation system...")

		// Get moderation configuration from config
		apiEndpoint := config.GetExternalURL("moderator")
		threshold := viper.GetFloat64("content_filtering.image_moderation.threshold")
		mode := viper.GetString("content_filtering.image_moderation.mode")
		timeout := time.Duration(viper.GetInt("content_filtering.image_moderation.timeout_seconds")) * time.Second
		checkInterval := time.Duration(viper.GetInt("content_filtering.image_moderation.check_interval_seconds")) * time.Second
		tempDir := config.GetPath("moderation")
		concurrency := viper.GetInt("content_filtering.image_moderation.concurrency")

		// Make sure temp directory exists
		if _, err := os.Stat(tempDir); os.IsNotExist(err) {
			if err := os.MkdirAll(tempDir, 0755); err != nil {
				logging.Infof("Failed to create moderation temp directory: %v", err)
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
			logging.Errorf("Failed to initialize image moderation: %v", err)
		} else {
			logging.Info("Image moderation system initialized successfully")
		}
	} else {
		logging.Info("Image moderation system is disabled")
	}

	// Initialize subscription manager with tiers from allowed_users
	subscription.InitGlobalManager(
		store,
		privateKey,
		dhtKey,
		settings.AllowedUsersSettings.Tiers,
	)
	logging.Info("Global subscription manager initialized successfully")

	// Batch update existing kind 11888 events on startup to ensure they reflect current config
	logging.Info("Updating existing kind 11888 events to reflect current configuration...")
	if manager := subscription.GetGlobalManager(); manager != nil {
		go func() {
			if err := manager.BatchUpdateAllSubscriptionEvents(); err != nil {
				logging.Errorf("Failed to update existing kind 11888 events on startup: %v", err)
			} else {
				logging.Info("Successfully updated existing kind 11888 events on startup")
			}
		}()
	}

	// Initialize daily free tier subscription renewal
	logging.Info("Initializing daily free tier subscription renewal...")
	subscription.InitDailyFreeSubscriptionRenewal()

	// Validate subscription mode configuration and ensure address availability
	if settings.AllowedUsersSettings.Mode == "subscription" {
		logging.Info("Relay is in subscription mode - validating Bitcoin address availability...")
		if manager := subscription.GetGlobalManager(); manager != nil {
			if err := validateSubscriptionModeStartup(manager, store); err != nil {
				logging.Fatal("Subscription mode validation failed", map[string]interface{}{
					"error": err,
				})
			}
		} else {
			logging.Fatal("Subscription manager not available for subscription mode validation")
		}
	} else {
		logging.Info("Relay not in subscription mode - skipping Bitcoin address validation", map[string]interface{}{
			"mode": settings.AllowedUsersSettings.Mode,
		})
	}

	// Initialize the global access control
	logging.Info("Initializing global access control...")
	if statsStore := store.GetStatsStore(); statsStore != nil {
		if err := ws.InitializeAccessControl(statsStore); err != nil {
			logging.Errorf("Failed to initialize access control: %v", err)
		} else {
			logging.Info("Global access control initialized successfully")
		}
	} else {
		logging.Warn("Warning: Statistics store not available, access control not initialized")
	}

	// Create and store kind 10411 event
	if err := kind10411.CreateKind10411Event(privateKey, publicKey, store); err != nil {
		logging.Errorf("Failed to create kind 10411 event: %v", err)
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

	logging.Infof("Host started with id: %s\n", host.ID())
	logging.Infof("Host started with address: %s\n", host.Addrs())

	syncDB, err := negentropy.InitSyncDB()
	if err != nil {
		logging.Fatalf("failed to connect database: %s", err.Error())
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
		logging.Infof("Self Relay: %+v\n", selfRelay)

		dhtServer := negentropy.DefaultDHTServer()
		defer dhtServer.Close()

		// this periodically syncs with other relays, and uploads user keys to dht
		uploadInterval := time.Hour * 2
		syncInterval := time.Hour * 3
		relayStore := negentropy.NewRelayStore(syncDB, dhtServer, host, store, uploadInterval, syncInterval)
		logging.Infof("Created relay store: %+v", relayStore)
	}

	// Register All Nostr Stream Handlers
	// Always register all specific handlers for registered kinds
	logging.Info("Registering all specific kind handlers...")
	nostr.RegisterHandler("kind/0", kind0.BuildKind0Handler(store, privateKey))
	nostr.RegisterHandler("kind/1", kind1.BuildKind1Handler(store))
	nostr.RegisterHandler("kind/1808", kind1808.BuildKind1808Handler(store))
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
	nostr.RegisterHandler("kind/16630", kind16630.BuildKind16630Handler(store))
	nostr.RegisterHandler("kind/10010", kind10010.BuildKind10010Handler(store))
	nostr.RegisterHandler("kind/19841", kind19841.BuildKind19841Handler(store))
	nostr.RegisterHandler("kind/19842", kind19842.BuildKind19842Handler(store))
	nostr.RegisterHandler("kind/19843", kind19843.BuildKind19843Handler(store))
	nostr.RegisterHandler("kind/117", kind117.BuildKind117Handler(store))
	nostr.RegisterHandler("kind/1063", kind1063.BuildKind1063Handler(store))

	// Always register universal handler for unregistered kinds
	nostr.RegisterHandler("universal", universal.BuildUniversalHandler(store))

	if viper.GetBool("event_filtering.allow_unregistered_kinds") {
		logging.Info("Universal handler registered for unregistered kinds (allow_unregistered_kinds=true)")
	} else {
		logging.Info("Universal handler registered but unregistered kinds are disabled (allow_unregistered_kinds=false)")
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
	if config.IsEnabled("web") {
		wg.Add(1)

		logging.Info("Starting with web server enabled")

		go func() {
			err = web.StartServer(store)

			if err != nil {
				logging.Info("Fatal error occurred in web server")
			}

			wg.Done()
		}()
	}

	// Nostr web sockets
	if config.IsEnabled("nostr") {
		wg.Add(1)

		logging.Info("Starting with legacy nostr proxy web server enabled")

		go func() {
			app := ws.BuildServer(store)
			err := ws.StartServer(app)

			if err != nil {
				logging.Info("Fatal error occurred in web server")
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

// validateSubscriptionModeStartup ensures that when the relay starts in subscription mode,
// all existing Kind 11888 events have Bitcoin addresses assigned and the wallet service is available
func validateSubscriptionModeStartup(manager *subscription.SubscriptionManager, store *badgerhold.BadgerholdStore) error {
	logging.Info("Starting subscription mode validation at startup...")

	// Step 1: Check wallet service connectivity using subscription package function
	walletHealthy, err := subscription.CheckWalletServiceHealth()
	if err != nil || !walletHealthy {
		return fmt.Errorf("wallet service is not available - cannot start in subscription mode: %v", err)
	}
	logging.Info("Wallet service connectivity verified")

	// Step 2: Check if there are existing Kind 11888 events without Bitcoin addresses
	statsStore := store.GetStatsStore()
	if statsStore == nil {
		return fmt.Errorf("statistics store not available")
	}

	usersWithoutAddresses, err := statsStore.CountUsersWithoutBitcoinAddresses()
	if err != nil {
		return fmt.Errorf("failed to count users without Bitcoin addresses: %v", err)
	}

	if usersWithoutAddresses == 0 {
		logging.Info("All existing users already have Bitcoin addresses assigned")
		return nil
	}

	logging.Info("Found users without Bitcoin addresses, starting migration", map[string]interface{}{
		"users_needing_addresses": usersWithoutAddresses,
	})

	// Step 3: Run the batch allocation to assign addresses to existing users
	// This will handle ensuring sufficient addresses are available
	if err := manager.AllocateBitcoinAddressesForExistingUsers(); err != nil {
		return fmt.Errorf("failed to allocate Bitcoin addresses for existing users: %v", err)
	}

	logging.Info("Successfully completed subscription mode startup validation", map[string]interface{}{
		"users_processed": usersWithoutAddresses,
	})

	return nil
}
