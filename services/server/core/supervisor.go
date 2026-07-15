package core

import (
	"context"
	"sync"
	"time"

	hsListener "github.com/HORNET-Storage/hdk-nostr-go/lib/connmgr/hyperswarm"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/sidecar"
	hsClient "github.com/hornet-storage/hornets-hyperswarm/clients/go/hyperswarm"
)

const (
	// sidecarPingInterval is how often the supervisor pings the sidecar to
	// detect hung (but not closed) connections. Outright process death is
	// detected immediately through the client's Done channel.
	sidecarPingInterval = 30 * time.Second

	// sidecarRetryBackoffMax caps the exponential backoff between recovery
	// attempts. Recovery retries forever; the relay is not useful on the DHT
	// without its sidecar, and the sidecar may come back at any time.
	sidecarRetryBackoffMax = 30 * time.Second
)

// sidecarSupervisor watches the hyperswarm sidecar connection and recovers
// the relay's DHT presence when the sidecar process dies, hangs, or
// restarts.
//
// Without supervision a dead sidecar leaves the relay looking healthy (web
// panel and websockets keep serving) while its P2P presence is silently
// gone: the DHT server, every registered protocol, and the service
// announcements all live inside the sidecar process.
type sidecarSupervisor struct {
	mu          sync.Mutex
	client      *hsClient.Client
	upnpCleanup func()
	closed      bool

	listener *hsListener.HyperswarmListener
	dhtSeed  string
}

// newSidecarSupervisor wraps the initial sidecar client and takes ownership
// of its UPnP cleanup function. Call Start once the listener has every
// protocol registered; call CleanupUPnP (usually deferred) on shutdown.
func newSidecarSupervisor(client *hsClient.Client, upnpCleanup func()) *sidecarSupervisor {
	return &sidecarSupervisor{client: client, upnpCleanup: upnpCleanup}
}

// Start launches the supervision loop. The listener must already have every
// protocol registered so a recovery can re-declare all of them. The loop
// exits when ctx is cancelled.
func (s *sidecarSupervisor) Start(ctx context.Context, listener *hsListener.HyperswarmListener, dhtSeed string) {
	s.mu.Lock()
	s.listener = listener
	s.dhtSeed = dhtSeed
	client := s.client
	s.mu.Unlock()

	go s.run(ctx, client)
}

// CleanupUPnP removes the currently held HyperDHT UPnP port mapping (if
// any) and prevents the supervisor from installing new mappings afterwards.
func (s *sidecarSupervisor) CleanupUPnP() {
	s.mu.Lock()
	cleanup := s.upnpCleanup
	s.upnpCleanup = nil
	s.closed = true
	s.mu.Unlock()

	if cleanup != nil {
		cleanup()
	}
}

func (s *sidecarSupervisor) run(ctx context.Context, client *hsClient.Client) {
	for {
		if !s.waitForLoss(ctx, client) {
			return
		}

		logging.Error("Hyperswarm sidecar connection lost - starting recovery", nil)

		recovered, ok := s.recover(ctx)
		if !ok {
			return
		}
		client = recovered
	}
}

// waitForLoss blocks until the sidecar connection is lost (true) or ctx is
// cancelled (false). Outright process death closes the client's Done
// channel; a hung sidecar is caught by periodic pings (bounded internally
// by the client's default request timeout), where a failure force-closes
// the client so Done fires on the next iteration.
func (s *sidecarSupervisor) waitForLoss(ctx context.Context, client *hsClient.Client) bool {
	ticker := time.NewTicker(sidecarPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-client.Done():
			return true
		case <-ticker.C:
			if _, err := client.Ping(); err != nil {
				logging.Warn("Hyperswarm sidecar ping failed - treating connection as lost", map[string]interface{}{
					"error": err.Error(),
				})
				_ = client.Close()
			}
		}
	}
}

// recover re-acquires a sidecar client and re-declares the DHT server plus
// every registered protocol, retrying forever with capped exponential
// backoff. It returns the new client and true on success, or false when ctx
// was cancelled. The DHT seed is deterministic, so the recovered server
// keeps the same public key and every advertised address stays valid.
func (s *sidecarSupervisor) recover(ctx context.Context) (*hsClient.Client, bool) {
	backoff := time.Second

	for attempt := 1; ; attempt++ {
		select {
		case <-ctx.Done():
			return nil, false
		default:
		}

		client, err := sidecar.GetClient()
		if err == nil {
			var serverID, publicKey string
			serverID, publicKey, err = s.listener.Reattach(client, s.dhtSeed)
			if err == nil {
				s.mu.Lock()
				s.client = client
				s.mu.Unlock()

				s.swapUPnP(client)

				logging.Info("Hyperswarm sidecar recovered", map[string]interface{}{
					"server_id":      serverID,
					"dht_public_key": publicKey,
					"attempts":       attempt,
				})
				return client, true
			}
		}

		logging.Errorf("Hyperswarm sidecar recovery attempt %d failed (retrying in %s): %v", attempt, backoff, err)

		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > sidecarRetryBackoffMax {
			backoff = sidecarRetryBackoffMax
		}
	}
}

// swapUPnP releases the previous HyperDHT port mapping and forwards the
// port reported by the recovered sidecar (the DHT port can change across
// sidecar restarts). No new mapping is installed once CleanupUPnP has run.
func (s *sidecarSupervisor) swapUPnP(client *hsClient.Client) {
	s.mu.Lock()
	old := s.upnpCleanup
	s.upnpCleanup = nil
	closed := s.closed
	s.mu.Unlock()

	if old != nil {
		old()
	}
	if closed {
		return
	}

	cleanup := forwardSidecarDHTPort(client)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cleanup()
		return
	}
	s.upnpCleanup = cleanup
	s.mu.Unlock()
}
