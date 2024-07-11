package proxy

import (
	"context"
	"encoding/json"

	"github.com/gofiber/contrib/websocket"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/nbd-wtf/go-nostr"
	"github.com/puzpuzpuz/xsync/v3"
)

type nip11RelayInfo struct {
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	Pubkey        string `json:"pubkey,omitempty"`
	Contact       string `json:"contact,omitempty"`
	SupportedNIPs []int  `json:"supported_nips,omitempty"`
	Software      string `json:"software,omitempty"`
	Version       string `json:"version,omitempty"`
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

type RelaySettings struct {
	Mode     string   `json:"mode"`
	Kinds    []string `json:"kinds"`
	Photos   []string `json:"photos"`
	Videos   []string `json:"videos"`
	GitNestr []string `json:"gitNestr"`
}
