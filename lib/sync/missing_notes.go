package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"
)

// MissingNoteRequest represents a request to retrieve a missing note
type MissingNoteRequest struct {
	EventID      string `json:"event_id"`      // ID of the missing event
	AuthorPubkey string `json:"author_pubkey"` // Pubkey of the event author
	DHTKey       string `json:"dht_key"`       // DHT key for the author's relay list
}

// MissingNoteResponse represents a response to a missing note request
type MissingNoteResponse struct {
	Event     *nostr.Event `json:"event"`     // The retrieved event, if found
	Found     bool         `json:"found"`     // Whether the event was found
	RelayURL  string       `json:"relay_url"` // URL of the relay where the event was found
	Timestamp int64        `json:"timestamp"` // Timestamp of when the event was found
}

// RetrieveMissingNote retrieves a missing note from relays associated with a user
// It uses the DHT to find the user's relay list, then queries those relays for the note
func RetrieveMissingNote(eventID string, authorPubkey string, relayStore *RelayStore, eventStore stores.Store) (*MissingNoteResponse, error) {
	log.Printf("Retrieving missing note %s from author %s", eventID, authorPubkey)

	// First check if we already have the event locally
	filter := nostr.Filter{
		IDs: []string{eventID},
	}
	localEvents, err := eventStore.QueryEvents(filter)
	if err == nil && len(localEvents) > 0 {
		log.Printf("Event %s found locally", eventID)
		return &MissingNoteResponse{
			Event:     localEvents[0],
			Found:     true,
			RelayURL:  "local",
			Timestamp: time.Now().Unix(),
		}, nil
	}

	// Get the DHT key for the author
	dhtKey, err := GetDHTKeyForPubkey(authorPubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to get DHT key for pubkey %s: %w", authorPubkey, err)
	}

	// Retrieve the relay list from DHT
	relayURLs, err := relayStore.RetrieveRelayList(dhtKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve relay list for DHT key %s: %w", dhtKey, err)
	}

	if len(relayURLs) == 0 {
		return nil, fmt.Errorf("no relays found for author %s", authorPubkey)
	}

	log.Printf("Found %d relays for author %s", len(relayURLs), authorPubkey)

	// Try to retrieve the note from each relay
	for _, relayURL := range relayURLs {
		event, err := retrieveEventFromRelay(eventID, relayURL)
		if err != nil {
			log.Printf("Error retrieving event %s from relay %s: %v", eventID, relayURL, err)
			continue
		}

		if event != nil {
			log.Printf("Event %s found at relay %s", eventID, relayURL)

			// Verify the event signature
			if ok, err := event.CheckSignature(); err != nil || !ok {
				log.Printf("Event %s has invalid signature: %v", eventID, err)
				continue
			}

			// Store the event locally for future reference
			if err := eventStore.StoreEvent(event); err != nil {
				log.Printf("Error storing event %s locally: %v", eventID, err)
			}

			return &MissingNoteResponse{
				Event:     event,
				Found:     true,
				RelayURL:  relayURL,
				Timestamp: time.Now().Unix(),
			}, nil
		}
	}

	return &MissingNoteResponse{
		Found:     false,
		Timestamp: time.Now().Unix(),
	}, nil
}

// retrieveEventFromRelay attempts to retrieve an event from a specific relay
func retrieveEventFromRelay(eventID string, relayURL string) (*nostr.Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to the relay
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to relay %s: %w", relayURL, err)
	}
	defer relay.Close()

	// Create a filter to request the specific event
	filter := nostr.Filter{
		IDs: []string{eventID},
	}

	// Subscribe to events matching the filter
	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to relay %s: %w", relayURL, err)
	}

	// Wait for events or timeout
	for {
		select {
		case ev := <-sub.Events:
			if ev.ID == eventID {
				return ev, nil
			}
		case <-sub.EndOfStoredEvents:
			// No more events from this relay
			return nil, nil
		case <-ctx.Done():
			// Timeout or cancellation
			return nil, ctx.Err()
		}
	}
}

// RequestMissingNoteFromRelay sends a request to another relay to retrieve a missing note
// This is used when a relay needs to request a note from another relay
func RequestMissingNoteFromRelay(request *MissingNoteRequest, targetRelayURL string) (*MissingNoteResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to the target relay
	relay, err := nostr.RelayConnect(ctx, targetRelayURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to relay %s: %w", targetRelayURL, err)
	}
	defer relay.Close()

	// Authenticate with the relay if needed (NIP-42)
	// This would be implemented based on the relay's authentication requirements

	// Create a filter to request the specific event
	filter := nostr.Filter{
		IDs: []string{request.EventID},
	}

	// Subscribe to events matching the filter
	sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to relay %s: %w", targetRelayURL, err)
	}

	// Wait for events or timeout
	for {
		select {
		case ev := <-sub.Events:
			if ev.ID == request.EventID {
				return &MissingNoteResponse{
					Event:     ev,
					Found:     true,
					RelayURL:  targetRelayURL,
					Timestamp: time.Now().Unix(),
				}, nil
			}
		case <-sub.EndOfStoredEvents:
			// No more events from this relay
			return &MissingNoteResponse{
				Found:     false,
				RelayURL:  targetRelayURL,
				Timestamp: time.Now().Unix(),
			}, nil
		case <-ctx.Done():
			// Timeout or cancellation
			return nil, ctx.Err()
		}
	}
}

// HandleMissingNoteRequest processes a request for a missing note
// This is called when a relay receives a request for a missing note
func HandleMissingNoteRequest(requestJSON []byte, eventStore stores.Store, relayStore *RelayStore) ([]byte, error) {
	// Parse the request
	var request MissingNoteRequest
	if err := json.Unmarshal(requestJSON, &request); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	// Try to retrieve the note
	response, err := RetrieveMissingNote(request.EventID, request.AuthorPubkey, relayStore, eventStore)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve missing note: %w", err)
	}

	// Marshal the response
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return responseJSON, nil
}
