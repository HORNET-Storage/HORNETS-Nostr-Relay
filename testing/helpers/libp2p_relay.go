// Package helpers provides utilities for integration testing the HORNETS relay
package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/multiformats/go-multiaddr"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic/download"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic/query"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic/upload"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/spf13/viper"
)

// TestLibp2pRelay represents a test relay instance with libp2p support
type TestLibp2pRelay struct {
	Store      *badgerhold.BadgerholdStore
	Host       host.Host
	Multiaddr  string // Full multiaddr including peer ID
	DataDir    string
	PrivateKey string
	PublicKey  string
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	running    bool
}

// TestLibp2pRelayConfig holds configuration for a test libp2p relay
type TestLibp2pRelayConfig struct {
	DataDir    string
	PrivateKey string // Optional - will generate if empty
}

// DefaultTestLibp2pConfig returns a default test configuration for libp2p relay
func DefaultTestLibp2pConfig() TestLibp2pRelayConfig {
	return TestLibp2pRelayConfig{
		DataDir:    "",
		PrivateKey: "",
	}
}

// NewTestLibp2pRelay creates and starts a new test libp2p relay
func NewTestLibp2pRelay(cfg TestLibp2pRelayConfig) (*TestLibp2pRelay, error) {
	// Create temp directory if not specified
	dataDir := cfg.DataDir
	if dataDir == "" {
		var err error
		dataDir, err = os.MkdirTemp("", "hornet-test-libp2p-relay-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
	}

	// Initialize config
	initTestLibp2pConfig(dataDir, cfg)

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

	// Deserialize private key
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

	// Create libp2p host
	libp2pPrivateKey, err := crypto.UnmarshalSecp256k1PrivateKey(privateKey.Serialize())
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to unmarshal libp2p private key: %w", err)
	}

	// Listen on random port
	listenAddr := "/ip4/127.0.0.1/udp/0/quic-v1"

	libp2pHost, err := libp2p.New(
		libp2p.Identity(libp2pPrivateKey),
		libp2p.ListenAddrStrings(listenAddr),
		libp2p.Transport(libp2pquic.NewTransport),
	)
	if err != nil {
		os.RemoveAll(dataDir)
		store.Cleanup()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Create context
	ctx, cancel := context.WithCancel(context.Background())

	relay := &TestLibp2pRelay{
		Store:      store,
		Host:       libp2pHost,
		DataDir:    dataDir,
		PrivateKey: privateKeyStr,
		PublicKey:  *serializedPublicKey,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Build multiaddr with peer ID
	for _, addr := range libp2pHost.Addrs() {
		relay.Multiaddr = fmt.Sprintf("%s/p2p/%s", addr.String(), libp2pHost.ID().String())
		break
	}

	// Register DAG handlers
	if err := relay.registerHandlers(ctx); err != nil {
		cancel()
		libp2pHost.Close()
		store.Cleanup()
		os.RemoveAll(dataDir)
		return nil, fmt.Errorf("failed to register handlers: %w", err)
	}

	relay.running = true
	return relay, nil
}

// registerHandlers registers the DAG handlers on the libp2p host
func (r *TestLibp2pRelay) registerHandlers(ctx context.Context) error {
	// Allow all uploads for testing
	canUpload := func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		return true
	}

	// No-op handler for received DAGs
	handleReceived := func(dag *merkle_dag.Dag, pubKey *string) {}

	// Allow all downloads for testing
	canDownload := func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool {
		return true
	}

	// Register upload handler
	upload.AddUploadHandlerForLibp2p(ctx, r.Host, r.Store, canUpload, handleReceived)

	// Register download handler
	download.AddDownloadHandler(r.Host, r.Store, canDownload)

	// Register query handler
	query.AddQueryHandler(r.Host, r.Store)

	return nil
}

// GetConnectionManager creates a connection manager connected to this relay
func (r *TestLibp2pRelay) GetConnectionManager(ctx context.Context) (*connmgr.GenericConnectionManager, error) {
	cm := connmgr.NewGenericConnectionManager()

	err := cm.ConnectWithLibp2p(ctx, "test-relay", r.Multiaddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to relay: %w", err)
	}

	return cm, nil
}

// Stop stops the test relay and cleans up resources
func (r *TestLibp2pRelay) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	// Cancel context
	r.cancel()

	// Close libp2p host
	if r.Host != nil {
		if err := r.Host.Close(); err != nil {
			logging.Errorf("Error closing libp2p host: %v", err)
		}
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
func (r *TestLibp2pRelay) Cleanup() error {
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

// WaitForReady waits for the relay to be ready to accept connections
func (r *TestLibp2pRelay) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Check if host is ready by verifying it has addresses
		if len(r.Host.Addrs()) > 0 {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for relay to be ready")
}

// initTestLibp2pConfig initializes viper config for libp2p testing
func initTestLibp2pConfig(dataDir string, cfg TestLibp2pRelayConfig) {
	viper.Reset()

	// Server config
	viper.Set("server.data_path", dataDir)

	// Generate keys if not provided
	if cfg.PrivateKey == "" {
		// Use a deterministic test key for reproducibility
		viper.Set("relay.private_key", "nsec1vl029mgpspedva04g90vltkh6fvh240zqtv9k0t9af8935ke9laqsnlfe5")
	} else {
		viper.Set("relay.private_key", cfg.PrivateKey)
	}

	// DAG upload settings - allow all for tests
	viper.Set("upload.enabled_uploads", []string{"all"})
	viper.Set("content_filtering.image_moderation.enabled", false)

	// Logging - minimal for tests
	viper.Set("logging.level", "error")
	viper.Set("logging.output", "stdout")

	// Initialize config system
	config.InitConfigForTesting()
}

// TestConnectionNotifier is a connection notifier for testing
type TestConnectionNotifier struct {
	ConnectedChan    chan struct{}
	DisconnectedChan chan struct{}
}

func NewTestConnectionNotifier() *TestConnectionNotifier {
	return &TestConnectionNotifier{
		ConnectedChan:    make(chan struct{}, 10),
		DisconnectedChan: make(chan struct{}, 10),
	}
}

func (n *TestConnectionNotifier) Connected(net network.Network, conn network.Conn) {
	select {
	case n.ConnectedChan <- struct{}{}:
	default:
	}
}

func (n *TestConnectionNotifier) Disconnected(net network.Network, conn network.Conn) {
	select {
	case n.DisconnectedChan <- struct{}{}:
	default:
	}
}

func (n *TestConnectionNotifier) Listen(net network.Network, multiaddr multiaddr.Multiaddr) {}

func (n *TestConnectionNotifier) ListenClose(net network.Network, multiaddr multiaddr.Multiaddr) {}
