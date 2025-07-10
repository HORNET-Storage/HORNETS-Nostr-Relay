package badgerhold

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/timshannon/badgerhold/v4"
)

// KindNIPMapping represents a mapping between a Nostr kind and its corresponding NIP
type KindNIPMapping struct {
	Kind string `badgerhold:"key"`
	NIP  string `json:"nip"`
}

// InitializeKindToNIPMappings initializes the BadgerDB with kind-to-NIP mappings
func (store *BadgerholdStore) InitializeKindToNIPMappings() error {
	// Check if mappings already exist
	var existingMapping KindNIPMapping
	err := store.Database.Get("1", &existingMapping)
	if err == nil {
		// Mappings already exist
		return nil
	}

	// Initialize default mappings based on your NIP documentation
	mappings := map[string]string{
		// NIP-01: Basic Protocol
		"0":  "1",  // Profile metadata
		"1":  "1",  // Short text note
		"2":  "1",  // Recommend relay (deprecated)

		// NIP-02: Contact List
		"3": "2", // Contact list

		// NIP-09: Event Deletion
		"5": "9", // Deletion request

		// NIP-18: Reposts
		"6":  "18", // Repost
		"16": "18", // Generic repost

		// NIP-25: Reactions
		"7": "25", // Reaction

		// NIP-58: Badges
		"8":     "58", // Badge award
		"30008": "58", // Profile badge
		"30009": "58", // Badge definition

		// NIP-23: Long-form Content
		"30023": "23", // Long-form content

		// NIP-51: Lists
		"10000": "51", // Mute list
		"10001": "51", // Pin list
		"30000": "51", // Categorized people list

		// NIP-56: Reporting
		"1984": "56", // Reporting

		// NIP-57: Lightning Zaps
		"9735": "57", // Zap receipt

		// NIP-65: Relay List Metadata
		"10002": "65", // Relay list metadata

		// NIP-84: Highlights
		"9802": "84", // Highlight

		// NIP-116: Event Paths
		"30079": "116", // Event paths

		// NIP-117: Double Ratchet DM
		"1060": "117", // Message event

		// NIP-118: Double Ratchet DM Invite
		"30078": "118", // Invite event

		// Custom HORNETS NIPs
		"117":   "888", // Blossom blob
		"10411": "888", // Subscription info
		"11888": "888", // Custom HORNETS protocol
		"555":   "555", // X-Nostr bridge

		// Additional kinds from your whitelist
		"10010": "51",  // Additional list type
		"10011": "51",  // Additional list type
		"10022": "51",  // Additional list type
		"9803":  "84",  // Additional highlight type
		"22242": "888", // Custom HORNETS kind
		"19841": "888", // Payment subscription
		"19842": "888", // Payment subscription
		"19843": "888", // Payment subscription
	}

	// Store mappings in BadgerDB
	for kind, nip := range mappings {
		mapping := KindNIPMapping{
			Kind: kind,
			NIP:  nip,
		}
		err := store.Database.Upsert(kind, mapping)
		if err != nil {
			return fmt.Errorf("failed to store kind-to-NIP mapping for kind %s: %v", kind, err)
		}
	}

	logging.Info("Kind-to-NIP mappings initialized successfully", map[string]interface{}{
		"mappings_count": len(mappings),
	})

	return nil
}

// GetNIPForKind returns the NIP number for a given kind
func (store *BadgerholdStore) GetNIPForKind(kind int) (int, error) {
	var mapping KindNIPMapping
	kindStr := strconv.Itoa(kind)
	
	err := store.Database.Get(kindStr, &mapping)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return 0, fmt.Errorf("no NIP mapping found for kind %d", kind)
		}
		return 0, fmt.Errorf("failed to get NIP for kind %d: %v", kind, err)
	}

	nip, err := strconv.Atoi(mapping.NIP)
	if err != nil {
		return 0, fmt.Errorf("invalid NIP number for kind %d: %v", kind, err)
	}

	return nip, nil
}

// GetSupportedNIPsFromKinds returns unique NIP numbers for given kinds
func (store *BadgerholdStore) GetSupportedNIPsFromKinds(kinds []string) ([]int, error) {
	nipSet := make(map[int]struct{})
	
	// Always include system-critical NIPs
	systemCriticalKinds := []int{555, 10411, 11888}
	for _, kind := range systemCriticalKinds {
		if nip, err := store.GetNIPForKind(kind); err == nil {
			nipSet[nip] = struct{}{}
		}
	}
	
	// Process user-configured kinds
	for _, kindStr := range kinds {
		// Remove "kind" prefix if present
		kindStr = strings.TrimPrefix(kindStr, "kind")
		
		kind, err := strconv.Atoi(kindStr)
		if err != nil {
			logging.Warn("Invalid kind number", map[string]interface{}{
				"kind_string": kindStr,
				"error":       err.Error(),
			})
			continue
		}
		
		nip, err := store.GetNIPForKind(kind)
		if err != nil {
			logging.Warn("No NIP mapping found for kind", map[string]interface{}{
				"kind": kind,
				"error": err.Error(),
			})
			continue
		}
		
		nipSet[nip] = struct{}{}
	}
	
	// Convert set to sorted slice
	nips := make([]int, 0, len(nipSet))
	for nip := range nipSet {
		nips = append(nips, nip)
	}
	sort.Ints(nips)
	
	return nips, nil
}

// AddKindToNIPMapping adds or updates a kind-to-NIP mapping
func (store *BadgerholdStore) AddKindToNIPMapping(kind int, nip int) error {
	kindStr := strconv.Itoa(kind)
	nipStr := strconv.Itoa(nip)
	
	mapping := KindNIPMapping{
		Kind: kindStr,
		NIP:  nipStr,
	}
	
	err := store.Database.Upsert(kindStr, mapping)
	if err != nil {
		return fmt.Errorf("failed to add kind-to-NIP mapping: %v", err)
	}
	
	logging.Info("Added kind-to-NIP mapping", map[string]interface{}{
		"kind": kind,
		"nip":  nip,
	})
	
	return nil
}