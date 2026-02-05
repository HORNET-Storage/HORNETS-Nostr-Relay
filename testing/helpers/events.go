// Package helpers provides utilities for integration testing the HORNETS relay
package helpers

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// TestKeyPair represents a key pair for testing
type TestKeyPair struct {
	PrivateKey string
	PublicKey  string
}

// GenerateKeyPair generates a new key pair for testing
func GenerateKeyPair() (*TestKeyPair, error) {
	sk := nostr.GeneratePrivateKey()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}
	return &TestKeyPair{
		PrivateKey: sk,
		PublicKey:  pk,
	}, nil
}

// CreateTextNote creates a kind 1 text note event
func CreateTextNote(kp *TestKeyPair, content string, tags ...nostr.Tag) (*nostr.Event, error) {
	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      1,
		Tags:      tags,
		Content:   content,
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateMetadata creates a kind 0 metadata event
func CreateMetadata(kp *TestKeyPair, name, about, picture string) (*nostr.Event, error) {
	metadata := map[string]string{
		"name":    name,
		"about":   about,
		"picture": picture,
	}
	content, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      0,
		Tags:      nil,
		Content:   string(content),
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateContactList creates a kind 3 contact list event
func CreateContactList(kp *TestKeyPair, contacts []string) (*nostr.Event, error) {
	var tags nostr.Tags
	for _, contact := range contacts {
		tags = append(tags, nostr.Tag{"p", contact})
	}

	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      3,
		Tags:      tags,
		Content:   "",
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateDeletionEvent creates a kind 5 deletion event
func CreateDeletionEvent(kp *TestKeyPair, eventIDs []string, reason string) (*nostr.Event, error) {
	var tags nostr.Tags
	for _, id := range eventIDs {
		tags = append(tags, nostr.Tag{"e", id})
	}

	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      5,
		Tags:      tags,
		Content:   reason,
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateReaction creates a kind 7 reaction event
func CreateReaction(kp *TestKeyPair, eventID, eventPubKey, reaction string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"e", eventID},
		{"p", eventPubKey},
	}

	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      7,
		Tags:      tags,
		Content:   reaction,
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateRepost creates a kind 6 repost event
func CreateRepost(kp *TestKeyPair, eventID, eventPubKey, relayURL string, originalEvent *nostr.Event) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"e", eventID, relayURL},
		{"p", eventPubKey},
	}

	// Content is the JSON-encoded original event
	content := ""
	if originalEvent != nil {
		eventJSON, err := json.Marshal(originalEvent)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal original event: %w", err)
		}
		content = string(eventJSON)
	}

	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      6,
		Tags:      tags,
		Content:   content,
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateGenericEvent creates an event of any kind
func CreateGenericEvent(kp *TestKeyPair, kind int, content string, tags nostr.Tags) (*nostr.Event, error) {
	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateReplaceableEvent creates a replaceable event (kinds 0, 3, or 10000-19999)
func CreateReplaceableEvent(kp *TestKeyPair, kind int, content string) (*nostr.Event, error) {
	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      kind,
		Tags:      nil,
		Content:   content,
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}

// CreateParameterizedReplaceableEvent creates a parameterized replaceable event (kinds 30000-39999)
func CreateParameterizedReplaceableEvent(kp *TestKeyPair, kind int, dTag, content string) (*nostr.Event, error) {
	tags := nostr.Tags{
		{"d", dTag},
	}

	event := &nostr.Event{
		PubKey:    kp.PublicKey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}
	if err := event.Sign(kp.PrivateKey); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}
	return event, nil
}
