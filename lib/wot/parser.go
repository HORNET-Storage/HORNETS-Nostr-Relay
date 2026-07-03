// Package wot implements a parser for the nostr-social-graph binary format
// and a BFS-based follow-distance calculator. It has no relay-specific
// dependencies — it works with raw bytes and pubkey strings only.
package wot

import (
	"encoding/hex"
	"fmt"
)

const (
	// supportedBinaryVersion is the binary format version this parser supports.
	// See nostr-social-graph/src/SocialGraphBinary.ts for the canonical spec.
	supportedBinaryVersion = 2

	// pubkeyByteLen is the length of a raw Nostr pubkey in bytes (32 bytes = 64 hex chars).
	pubkeyByteLen = 32
)

// FollowGraph holds the adjacency list parsed from a nostr-social-graph binary.
// Each entry maps an internal node ID to its set of followed node IDs.
type FollowGraph struct {
	// idToPubkey maps internal IDs to hex-encoded pubkeys.
	idToPubkey map[uint32]string

	// pubkeyToID maps hex-encoded pubkeys to internal IDs.
	pubkeyToID map[string]uint32

	// followedBy maps an internal ID to the set of IDs it follows.
	followedBy map[uint32][]uint32
}

// ParseBinary decodes a nostr-social-graph v2 binary into a FollowGraph.
// The binary format is:
//
//	[varint] version
//	[varint] uniqueIds count
//	  for each: [32 bytes] pubkey, [varint] internal ID
//	[varint] follow lists count
//	  for each: [varint] owner ID, [varint] timestamp, [varint] count, then count x [varint] target ID
//	[varint] mute lists count (skipped — mutes are not used for follow-distance)
func ParseBinary(data []byte) (*FollowGraph, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("wot: empty binary data")
	}

	offset := 0

	// Version
	version, n, err := decodeVarint(data, offset)
	if err != nil {
		return nil, fmt.Errorf("wot: failed to read version: %w", err)
	}
	offset += n
	if version != supportedBinaryVersion {
		return nil, fmt.Errorf("wot: unsupported binary version %d (expected %d)", version, supportedBinaryVersion)
	}

	// Unique IDs
	idsCount, n, err := decodeVarint(data, offset)
	if err != nil {
		return nil, fmt.Errorf("wot: failed to read uniqueIds count: %w", err)
	}
	offset += n

	g := &FollowGraph{
		idToPubkey: make(map[uint32]string, idsCount),
		pubkeyToID: make(map[string]uint32, idsCount),
		followedBy: make(map[uint32][]uint32),
	}

	for i := uint32(0); i < idsCount; i++ {
		// Read 32 raw bytes, convert to hex string
		if offset+pubkeyByteLen > len(data) {
			return nil, fmt.Errorf("wot: unexpected end of data reading pubkey %d", i)
		}
		pubkeyHex := hex.EncodeToString(data[offset : offset+pubkeyByteLen])
		offset += pubkeyByteLen

		id, n, err := decodeVarint(data, offset)
		if err != nil {
			return nil, fmt.Errorf("wot: failed to read id for pubkey %d: %w", i, err)
		}
		offset += n

		g.idToPubkey[id] = pubkeyHex
		g.pubkeyToID[pubkeyHex] = id
	}

	// Follow lists
	followListsCount, n, err := decodeVarint(data, offset)
	if err != nil {
		return nil, fmt.Errorf("wot: failed to read follow lists count: %w", err)
	}
	offset += n

	for i := uint32(0); i < followListsCount; i++ {
		ownerID, n, err := decodeVarint(data, offset)
		if err != nil {
			return nil, fmt.Errorf("wot: failed to read follow list owner %d: %w", i, err)
		}
		offset += n

		// Timestamp (skip — not needed for distance calculation)
		_, n, err = decodeVarint(data, offset)
		if err != nil {
			return nil, fmt.Errorf("wot: failed to read follow list timestamp %d: %w", i, err)
		}
		offset += n

		followCount, n, err := decodeVarint(data, offset)
		if err != nil {
			return nil, fmt.Errorf("wot: failed to read follow count %d: %w", i, err)
		}
		offset += n

		targets := make([]uint32, 0, followCount)
		for j := uint32(0); j < followCount; j++ {
			targetID, n, err := decodeVarint(data, offset)
			if err != nil {
				return nil, fmt.Errorf("wot: failed to read follow target %d/%d: %w", i, j, err)
			}
			offset += n
			targets = append(targets, targetID)
		}

		g.followedBy[ownerID] = targets
	}

	// Mute lists are present but not needed for follow-distance. Skip gracefully.
	// (No need to parse — the graph is complete for distance calculations.)

	return g, nil
}

// GetFollowDistance computes the shortest follow-hop distance from rootPubkey
// to targetPubkey using BFS. Returns -1 if the target is unreachable.
func (g *FollowGraph) GetFollowDistance(rootPubkey, targetPubkey string) int {
	rootID, rootOK := g.pubkeyToID[rootPubkey]
	targetID, targetOK := g.pubkeyToID[targetPubkey]

	if !rootOK || !targetOK {
		return -1
	}
	if rootID == targetID {
		return 0
	}

	// BFS from root
	visited := make(map[uint32]bool)
	visited[rootID] = true

	queue := []uint32{rootID}
	distance := 0

	for len(queue) > 0 {
		distance++
		nextQueue := make([]uint32, 0, len(queue)*2)

		for _, current := range queue {
			for _, neighbor := range g.followedBy[current] {
				if neighbor == targetID {
					return distance
				}
				if !visited[neighbor] {
					visited[neighbor] = true
					nextQueue = append(nextQueue, neighbor)
				}
			}
		}

		queue = nextQueue
	}

	return -1 // Unreachable
}

// ComputeAllDistances runs BFS from rootPubkey and returns a map of
// pubkey → follow distance for all reachable nodes, up to maxDistance.
// This is useful for caching: compute once, look up many times.
func (g *FollowGraph) ComputeAllDistances(rootPubkey string, maxDistance int) map[string]int {
	rootID, ok := g.pubkeyToID[rootPubkey]
	if !ok {
		return nil
	}

	distances := make(map[string]int)

	// Root is distance 0
	if pubkey, exists := g.idToPubkey[rootID]; exists {
		distances[pubkey] = 0
	}

	visited := make(map[uint32]bool)
	visited[rootID] = true

	queue := []uint32{rootID}

	for distance := 1; distance <= maxDistance && len(queue) > 0; distance++ {
		nextQueue := make([]uint32, 0, len(queue)*2)

		for _, current := range queue {
			for _, neighbor := range g.followedBy[current] {
				if !visited[neighbor] {
					visited[neighbor] = true
					nextQueue = append(nextQueue, neighbor)

					if pubkey, exists := g.idToPubkey[neighbor]; exists {
						distances[pubkey] = distance
					}
				}
			}
		}

		queue = nextQueue
	}

	return distances
}

// decodeVarint reads a variable-length unsigned integer from data at the given offset.
// Returns the decoded value, number of bytes consumed, and any error.
func decodeVarint(data []byte, offset int) (uint32, int, error) {
	var value uint32
	var shift uint
	bytesRead := 0

	for i := offset; i < len(data); i++ {
		b := data[i]
		value |= uint32(b&0x7F) << shift
		bytesRead++

		if b&0x80 == 0 {
			return value, bytesRead, nil
		}

		shift += 7
		if shift >= 35 {
			return 0, 0, fmt.Errorf("wot: varint overflow at offset %d", offset)
		}
	}

	return 0, 0, fmt.Errorf("wot: unexpected end of data reading varint at offset %d", offset)
}
