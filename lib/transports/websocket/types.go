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
	SupportedNIPs   []int            `json:"supported_nips,omitempty"`
	Software        string           `json:"software,omitempty"`
	Version         string           `json:"version,omitempty"`
	HornetExtension *HornetExtension `json:"hornet_extension,omitempty"` // custom extension for p2p context
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
