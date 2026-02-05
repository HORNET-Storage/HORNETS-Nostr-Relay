package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
)

// TODO: maybe we should move this into a different package since we use it in the sync package as well
// It certainly shouldn't be here, that's for sure
type NIP11RelayInfo struct {
	Name            string           `json:"name,omitempty"`
	Description     string           `json:"description,omitempty"`
	Pubkey          string           `json:"pubkey,omitempty"`
	Contact         string           `json:"contact,omitempty"`
	Icon            string           `json:"icon,omitempty"`
	SupportedNIPs   []int            `json:"supported_nips,omitempty"`
	Software        string           `json:"software,omitempty"`
	Version         string           `json:"version,omitempty"`
	BasePort        int              `json:"base_port,omitempty"`        // Base port for service offset calculations
	Services        RelayServices    `json:"services,omitempty"`         // External/non-offset service endpoints
	HornetExtension *HornetExtension `json:"hornet_extension,omitempty"` // custom extension for p2p context
}

// Port offset constants from Nostr base port
// Clients should use these offsets to calculate service ports
const (
	PortOffsetNostr    = 0 // Base port (Nostr WebSocket)
	PortOffsetHornets  = 1 // libp2p/QUIC for DAG transfers
	PortOffsetPanel    = 2 // HTTP admin panel
	PortOffsetReactDev = 3 // React development server
	PortOffsetWallet   = 4 // Wallet service
	PortOffsetBlossom  = 5 // Blossom media storage
)

// RelayServices is a dynamic map of service name to endpoint configuration
// Built-in services (hornets, panel, blossom) use fixed offsets from base_port
// External services like airlock are advertised here with their actual endpoints
type RelayServices map[string]*ServiceEndpoint

// ServiceEndpoint describes how to connect to a specific service
// For libp2p services, pubkey is provided so clients can derive the peer ID
type ServiceEndpoint struct {
	Host   string `json:"host,omitempty"`   // Hostname/IP if different from relay (for external services)
	Port   int    `json:"port"`             // Port number for the service
	Path   string `json:"path,omitempty"`   // Optional URL path (for HTTP services)
	Pubkey string `json:"pubkey,omitempty"` // secp256k1 public key (for libp2p services - client derives peer ID)
}

type HornetExtension struct {
	LibP2PID    string    `json:"libp2p_id"`
	LibP2PAddrs []string  `json:"libp2p_addrs"`
	Signature   string    `json:"signature"`
	LastUpdated time.Time `json:"last_updated"`
}

type Message struct {
	MessageType string          `json:"messageType"`
	Event       json.RawMessage `json:"event"`
}

type ReqEnvelope struct {
	SubscriptionID string
	nostr.Filter
}

type Event interface {
	HandleEvent(c *websocket.Conn, ctx context.Context, host *host.Host) error // Adapt HostType accordingly
}

type Subscription struct {
	filters nostr.Filters
	cancel  context.CancelFunc
}

type ListenerData struct {
	authenticated bool
	challenge     string
	subscriptions *xsync.MapOf[string, *Subscription]
}

type EventMessage struct {
	Event nostr.Event // Adapted for the specific event structure you're using
}
