// Package helpers provides utilities for integration testing the HORNETS relay
package helpers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/blossom"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/filter"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind0"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind1"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind16629"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind3"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind5"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind7"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind72"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/universal"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"

	nostrHandlers "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BlossomApp holds reference to blossom HTTP server
var blossomApp *fiber.App

// TestRelay represents a test relay instance
type TestRelay struct {
	Store       *badgerhold.BadgerholdStore
	App         *fiber.App
	Port        int
	BlossomPort int // Port for blossom/web services
	URL         string
	DataDir     string
	PrivateKey  string
	PublicKey   string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.Mutex
	running     bool
}

// TestRelayConfig holds configuration for a test relay
type TestRelayConfig struct {
	Port           int
	DataDir        string
	PrivateKey     string // Optional - will generate if empty
	EnableAuth     bool
	AllowedKinds   []int
	ModerationMode string // strict, moderate, relaxed
}

// DefaultTestConfig returns a default test configuration
func DefaultTestConfig() TestRelayConfig {
	return TestRelayConfig{
		Port:           0, // Will find available port
		DataDir:        "",
		PrivateKey:     "",
		EnableAuth:     false,
		AllowedKinds:   []int{0, 1, 3, 5, 6, 7, 8, 16},
		ModerationMode: "relaxed",
	}
}

// NewTestRelay creates and starts a new test relay
func NewTestRelay(cfg TestRelayConfig) (*TestRelay, error) {
	// Create temp directory if not specified
	dataDir := cfg.DataDir
	if dataDir == "" {
		var err error
		dataDir, err = os.MkdirTemp("", "hornet-test-relay-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	// Find available port if not specified
	port := cfg.Port
	if port == 0 {
		var err error
		port, err = findAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("failed to find available port: %w", err)
		}
	}

	// Initialize config
	initTestConfig(dataDir, port, cfg)

	// Initialize logging (suppress for tests)
	logging.InitLogger()

	// Initialize store
	storePath := filepath.Join(dataDir, "store")
	statsPath := filepath.Join(dataDir, "stats.db")
	store, err := badgerhold.InitStore(storePath, statsPath)
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	// Get or generate private key
	privateKeyStr := cfg.PrivateKey
	if privateKeyStr == "" {
		privateKeyStr = viper.GetString("relay.private_key")
	}

	// Deserialize private key for handlers that need it
	privateKey, publicKey, err := signing.DeserializePrivateKey(privateKeyStr)
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to deserialize private key: %w", err)
	}

	// Get serialized public key
	serializedPublicKey, err := signing.SerializePublicKey(publicKey)
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to serialize public key: %w", err)
	}

	// Register handlers
	registerTestHandlers(store, privateKeyStr, privateKey)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())

	relay := &TestRelay{
		Store:      store,
		Port:       port,
		URL:        fmt.Sprintf("ws://127.0.0.1:%d", port),
		DataDir:    dataDir,
		PrivateKey: privateKeyStr,
		PublicKey:  *serializedPublicKey,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start the WebSocket server
	if err := relay.start(); err != nil {
		cancel()
		store.Cleanup()
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to start relay: %w", err)
	}

	return relay, nil
}

// start starts the relay server
func (r *TestRelay) start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return nil
	}

	// Build WebSocket server
	r.App = websocket.BuildServer(r.Store)

	// Start server in background
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		addr := fmt.Sprintf("127.0.0.1:%d", r.Port)
		if err := r.App.Listen(addr); err != nil {
			// Ignore shutdown errors
			if err.Error() != "server closed" {
				logging.Errorf("Test relay error: %v", err)
			}
		}
	}()

	// Wait for server to be ready
	if err := waitForServer(r.URL, 5*time.Second); err != nil {
		return err
	}

	r.running = true
	return nil
}

// startWithBlossom starts the relay server with a separate blossom HTTP server
func (r *TestRelay) startWithBlossom(blossomPort int, store stores.Store) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return nil
	}

	// Build WebSocket server
	r.App = websocket.BuildServer(r.Store)

	// Start WebSocket server in background
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		addr := fmt.Sprintf("127.0.0.1:%d", r.Port)
		if err := r.App.Listen(addr); err != nil {
			if err.Error() != "server closed" {
				logging.Errorf("Test relay error: %v", err)
			}
		}
	}()

	// Build and start Blossom HTTP server
	blossomApp = fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// CORS for blossom
	blossomApp.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, DELETE, PUT, OPTIONS",
	}))

	// Setup blossom routes
	blossomServer := blossom.NewServer(store)
	blossomServer.SetupRoutes(blossomApp)

	// Start Blossom server in background
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		addr := fmt.Sprintf("127.0.0.1:%d", blossomPort)
		if err := blossomApp.Listen(addr); err != nil {
			if err.Error() != "server closed" {
				logging.Errorf("Blossom server error: %v", err)
			}
		}
	}()

	// Wait for WebSocket server to be ready
	if err := waitForServer(r.URL, 5*time.Second); err != nil {
		return err
	}

	// Wait for Blossom HTTP server to be ready
	if err := waitForHTTPServer(fmt.Sprintf("http://127.0.0.1:%d", blossomPort), 5*time.Second); err != nil {
		return err
	}

	r.running = true
	return nil
}

// Stop stops the test relay and cleans up resources
func (r *TestRelay) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	// Cancel context
	r.cancel()

	// Shutdown WebSocket server
	if r.App != nil {
		if err := r.App.Shutdown(); err != nil {
			logging.Errorf("Error shutting down test relay: %v", err)
		}
	}

	// Shutdown Blossom HTTP server if running
	if blossomApp != nil {
		if err := blossomApp.Shutdown(); err != nil {
			logging.Errorf("Error shutting down blossom server: %v", err)
		}
		blossomApp = nil
	}

	// Wait for goroutines
	r.wg.Wait()

	// Close database
	if r.Store != nil {
		if err := r.Store.Cleanup(); err != nil {
			logging.Errorf("Error cleaning up store: %v", err)
		}
	}

	r.running = false
	return nil
}

// Cleanup stops the relay and removes all test data
func (r *TestRelay) Cleanup() error {
	if err := r.Stop(); err != nil {
		return err
	}

	// Remove test data directory
	if r.DataDir != "" {
		if err := os.RemoveAll(r.DataDir); err != nil {
			return fmt.Errorf("failed to remove test data: %w", err)
		}
	}

	return nil
}

// Connect creates a new client connection to the test relay
func (r *TestRelay) Connect(ctx context.Context) (*nostr.Relay, error) {
	relay, err := nostr.RelayConnect(ctx, r.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to test relay: %w", err)
	}
	return relay, nil
}

// initTestConfig initializes viper config for testing
func initTestConfig(dataDir string, port int, cfg TestRelayConfig) {
	viper.Reset()

	// Server config
	viper.Set("server.data_path", dataDir)
	viper.Set("server.port", port-1) // Base port (nostr is +1)
	viper.Set("server.bind_address", "127.0.0.1")
	viper.Set("server.nostr", true)
	viper.Set("server.web", false)
	viper.Set("server.hornets", false)

	// Relay config
	viper.Set("relay.name", "Test Relay")
	viper.Set("relay.description", "Integration test relay")
	viper.Set("relay.supported_nips", []int{1, 2, 9, 11, 12, 15, 16, 20, 22, 33, 40, 42})

	// Generate keys if not provided
	if cfg.PrivateKey == "" {
		// Use a deterministic test key for reproducibility
		viper.Set("relay.private_key", "nsec1vl029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5")
	} else {
		viper.Set("relay.private_key", cfg.PrivateKey)
	}

	// Derive and set public key from private key (needed for NIP-11 peer ID derivation)
	privKeyStr := viper.GetString("relay.private_key")
	if _, pubKey, err := signing.DeserializePrivateKey(privKeyStr); err == nil {
		if serializedPubKey, err := signing.SerializePublicKey(pubKey); err == nil {
			viper.Set("relay.public_key", *serializedPubKey)
		}
	}

	// Event filtering
	viper.Set("event_filtering.allow_unregistered_kinds", true) // Allow any kind for tests
	viper.Set("event_filtering.registered_kinds", cfg.AllowedKinds)
	viper.Set("event_filtering.moderation_mode", cfg.ModerationMode)

	// Build kind whitelist from allowed kinds
	kindWhitelist := make([]string, len(cfg.AllowedKinds))
	for i, k := range cfg.AllowedKinds {
		kindWhitelist[i] = fmt.Sprintf("kind%d", k)
	}
	viper.Set("event_filtering.kind_whitelist", kindWhitelist)

	// Content filtering - disabled for tests
	viper.Set("content_filtering.text_filter.enabled", false)
	viper.Set("content_filtering.image_moderation.enabled", false)

	// Allowed users - public for tests
	viper.Set("allowed_users.mode", "public")
	viper.Set("allowed_users.read", "all_users")
	viper.Set("allowed_users.write", "all_users")

	// Logging - minimal for tests
	viper.Set("logging.level", "error")
	viper.Set("logging.output", "stdout")

	// Initialize config system
	config.InitConfigForTesting()
}

// registerTestHandlers registers all Nostr handlers for testing
func registerTestHandlers(store *badgerhold.BadgerholdStore, privateKeyStr string, privateKey *btcec.PrivateKey) {
	// Clear any existing handlers
	nostrHandlers.ClearHandlers()

	// Register all handlers
	nostrHandlers.RegisterHandler("kind/0", kind0.BuildKind0Handler(store, privateKey))
	nostrHandlers.RegisterHandler("kind/1", kind1.BuildKind1Handler(store))
	nostrHandlers.RegisterHandler("kind/3", kind3.BuildKind3Handler(store))
	nostrHandlers.RegisterHandler("kind/5", kind5.BuildKind5Handler(store))
	nostrHandlers.RegisterHandler("kind/72", kind72.BuildKind72Handler(store))
	nostrHandlers.RegisterHandler("kind/7", kind7.BuildKind7Handler(store))
	nostrHandlers.RegisterHandler("kind/16629", kind16629.BuildKind16629Handler(store))

	// Universal handler for other kinds
	nostrHandlers.RegisterHandler("universal", universal.BuildUniversalHandler(store))

	// Filter handler for queries (REQ)
	nostrHandlers.RegisterHandler("filter", filter.BuildFilterHandler(store))
}

// NewTestRelayWithConfig creates a test relay with custom config initialization
// Use this when tests need specific viper config settings beyond the defaults
func NewTestRelayWithConfig(cfg TestRelayConfig, configInit func(dataDir string, port int, cfg TestRelayConfig)) (*TestRelay, error) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		var err error
		dataDir, err = os.MkdirTemp("", "hornet-test-relay-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	port := cfg.Port
	if port == 0 {
		var err error
		port, err = findAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("failed to find available port: %w", err)
		}
	}

	// Run custom config initialization
	configInit(dataDir, port, cfg)

	// Initialize logging
	logging.InitLogger()

	// Initialize store
	storePath := filepath.Join(dataDir, "store")
	statsPath := filepath.Join(dataDir, "stats.db")
	store, err := badgerhold.InitStore(storePath, statsPath)
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	privateKeyStr := cfg.PrivateKey
	if privateKeyStr == "" {
		privateKeyStr = viper.GetString("relay.private_key")
	}

	privateKey, publicKey, err := signing.DeserializePrivateKey(privateKeyStr)
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to deserialize private key: %w", err)
	}

	serializedPublicKey, err := signing.SerializePublicKey(publicKey)
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to serialize public key: %w", err)
	}

	registerTestHandlers(store, privateKeyStr, privateKey)

	ctx, cancel := context.WithCancel(context.Background())

	relay := &TestRelay{
		Store:      store,
		Port:       port,
		URL:        fmt.Sprintf("ws://127.0.0.1:%d", port),
		DataDir:    dataDir,
		PrivateKey: privateKeyStr,
		PublicKey:  *serializedPublicKey,
		ctx:        ctx,
		cancel:     cancel,
	}

	if err := relay.start(); err != nil {
		cancel()
		store.Cleanup()
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to start relay: %w", err)
	}

	return relay, nil
}

// NewTestRelayWithServices creates a test relay with web/blossom config flags set
func NewTestRelayWithServices(cfg TestRelayConfig) (*TestRelay, error) {
	return NewTestRelayWithConfig(cfg, func(dataDir string, port int, cfg TestRelayConfig) {
		initTestConfig(dataDir, port, cfg)
		viper.Set("server.web", true)
		viper.Set("server.services.web.port", port+2)
		viper.Set("server.services.blossom.enabled", true)
		viper.Set("server.services.blossom.port", port+2)
		config.InitConfigForTesting()
	})
}

// NewTestRelayWithHornets creates a test relay with hornets config flags set
func NewTestRelayWithHornets(cfg TestRelayConfig) (*TestRelay, error) {
	return NewTestRelayWithConfig(cfg, func(dataDir string, port int, cfg TestRelayConfig) {
		initTestConfig(dataDir, port, cfg)
		viper.Set("server.hornets", true)
		viper.Set("server.services.hornets.port", port)
		config.InitConfigForTesting()
	})
}

// NewTestRelayWithAirlock creates a test relay with airlock config flags set
func NewTestRelayWithAirlock(cfg TestRelayConfig) (*TestRelay, error) {
	return NewTestRelayWithConfig(cfg, func(dataDir string, port int, cfg TestRelayConfig) {
		initTestConfig(dataDir, port, cfg)
		viper.Set("server.services.airlock.enabled", true)
		viper.Set("server.services.airlock.port", port+3)
		viper.Set("server.services.airlock.pubkey", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")
		config.InitConfigForTesting()
	})
}

// NewTestRelayWithBlossom creates a test relay with blossom service enabled
// This starts both the WebSocket server and a separate HTTP server for blossom routes
func NewTestRelayWithBlossom(cfg TestRelayConfig) (*TestRelay, error) {
	dataDir := cfg.DataDir
	if dataDir == "" {
		var err error
		dataDir, err = os.MkdirTemp("", "hornet-test-relay-blossom-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	// Find available ports (we need two: one for websocket, one for blossom HTTP)
	wsPort := cfg.Port
	if wsPort == 0 {
		var err error
		wsPort, err = findAvailablePort()
		if err != nil {
			return nil, fmt.Errorf("failed to find available port: %w", err)
		}
	}

	blossomPort, err := findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find available blossom port: %w", err)
	}

	initTestConfigWithBlossom(dataDir, wsPort, blossomPort, cfg)

	return createTestRelayWithBlossomFromConfig(dataDir, wsPort, blossomPort, cfg)
}

// initTestConfigWithBlossom initializes viper config with blossom service
func initTestConfigWithBlossom(dataDir string, wsPort int, blossomPort int, cfg TestRelayConfig) {
	initTestConfig(dataDir, wsPort, cfg)

	// Enable blossom service
	viper.Set("server.web", true)
	viper.Set("server.services.blossom.enabled", true)
	viper.Set("server.services.blossom.port", blossomPort)
	viper.Set("server.services.blossom.path", "/blossom")

	// Set allowed MIME types for testing
	viper.Set("data_filtering.allowed_mime_types", []string{
		"text/plain",
		"text/plain; charset=utf-8",
		"application/octet-stream",
		"application/json",
		"image/png",
		"image/jpeg",
	})

	// Re-initialize config after changes
	config.InitConfigForTesting()
}

// createTestRelayWithBlossomFromConfig creates a test relay with blossom HTTP server
func createTestRelayWithBlossomFromConfig(dataDir string, wsPort int, blossomPort int, cfg TestRelayConfig) (*TestRelay, error) {
	// Initialize logging (suppress for tests)
	logging.InitLogger()

	// Initialize store
	storePath := filepath.Join(dataDir, "store")
	statsPath := filepath.Join(dataDir, "stats.db")
	store, err := badgerhold.InitStore(storePath, statsPath)
	if err != nil {
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to initialize store: %w", err)
	}

	// Get or generate private key
	privateKeyStr := cfg.PrivateKey
	if privateKeyStr == "" {
		privateKeyStr = viper.GetString("relay.private_key")
	}

	// Deserialize private key for handlers that need it
	privateKey, publicKey, err := signing.DeserializePrivateKey(privateKeyStr)
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to deserialize private key: %w", err)
	}

	// Get serialized public key
	serializedPublicKey, err := signing.SerializePublicKey(publicKey)
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to serialize public key: %w", err)
	}

	// Register handlers
	registerTestHandlers(store, privateKeyStr, privateKey)

	// Create context
	ctx, cancel := context.WithCancel(context.Background())

	relay := &TestRelay{
		Store:       store,
		Port:        wsPort,
		BlossomPort: blossomPort,
		URL:         fmt.Sprintf("ws://127.0.0.1:%d", wsPort),
		DataDir:     dataDir,
		PrivateKey:  privateKeyStr,
		PublicKey:   *serializedPublicKey,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start the WebSocket server
	if err := relay.startWithBlossom(blossomPort, store); err != nil {
		cancel()
		store.Cleanup()
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to start relay: %w", err)
	}

	return relay, nil
}

// findAvailablePort finds an available TCP port
func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// waitForServer waits for the server to be ready
func waitForServer(url string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for server to start")
		case <-ticker.C:
			relay, err := nostr.RelayConnect(ctx, url)
			if err == nil {
				relay.Close()
				return nil
			}
		}
	}
}

// waitForHTTPServer waits for an HTTP server to be ready
func waitForHTTPServer(url string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{Timeout: 1 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for HTTP server to start")
		case <-ticker.C:
			resp, err := client.Get(url)
			if err == nil {
				resp.Body.Close()
				return nil
			}
		}
	}
}
