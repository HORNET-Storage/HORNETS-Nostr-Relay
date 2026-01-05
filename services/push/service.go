package push

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/nbd-wtf/go-nostr"
)

// PushService manages push notifications
type PushService struct {
	store      stores.Store
	config     *types.PushNotificationConfig
	queue      chan *NotificationTask
	workers    []*Worker
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	mutex      sync.RWMutex
	apnsClient APNSClient
	fcmClient  FCMClient
	isRunning  bool
	nameCache  map[string]string // Cache for author names (pubkey -> name)
	cacheMutex sync.RWMutex       // Mutex for cache access
}

// NotificationTask represents a push notification task
type NotificationTask struct {
	Pubkey      string
	Event       *nostr.Event
	DeviceToken string
	Platform    string
	Message     *PushMessage
	Attempts    int
}

// PushMessage represents the formatted push notification message
type PushMessage struct {
	Title    string
	Body     string
	Badge    int
	Sound    string
	Category string
	Data     map[string]interface{}
}

// APNSClient interface for Apple Push Notification service
type APNSClient interface {
	SendNotification(deviceToken string, message *PushMessage) error
}

// FCMClient interface for Firebase Cloud Messaging
type FCMClient interface {
	SendNotification(deviceToken string, message *PushMessage) error
}

// NewPushService creates a new push notification service
func NewPushService(store stores.Store) (*PushService, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get config: %w", err)
	}

	if !cfg.PushNotifications.Enabled {
		logging.Infof("Push notifications are disabled")
		return nil, nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	service := &PushService{
		store:     store,
		config:    &cfg.PushNotifications,
		queue:     make(chan *NotificationTask, cfg.PushNotifications.Service.QueueSize),
		ctx:       ctx,
		cancel:    cancel,
		nameCache: make(map[string]string),
	}

	// Initialize APNs client if enabled
	if cfg.PushNotifications.APNS.Enabled {
		apnsClient, err := NewAPNSClient(&cfg.PushNotifications.APNS)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize APNs client: %w", err)
		}
		service.apnsClient = apnsClient
	}

	// Initialize FCM client if enabled
	if cfg.PushNotifications.FCM.Enabled {
		fcmClient, err := NewFCMClient(&cfg.PushNotifications.FCM)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize FCM client: %w", err)
		}
		service.fcmClient = fcmClient
	}

	return service, nil
}

// Start starts the push notification service
func (ps *PushService) Start() error {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	if ps.isRunning {
		return fmt.Errorf("push service is already running")
	}

	logging.Infof("Starting push notification service with %d workers", ps.config.Service.WorkerCount)

	// Start workers
	for i := 0; i < ps.config.Service.WorkerCount; i++ {
		worker := NewWorker(i, ps.queue, ps)
		ps.workers = append(ps.workers, worker)
		ps.wg.Add(1)
		go worker.Start(&ps.wg)
	}

	ps.isRunning = true
	logging.Infof("Push notification service started successfully")
	return nil
}

// Stop stops the push notification service
func (ps *PushService) Stop() {
	ps.mutex.Lock()
	defer ps.mutex.Unlock()

	if !ps.isRunning {
		return
	}

	logging.Infof("Stopping push notification service...")

	// Signal cancellation
	ps.cancel()

	// Close the queue to signal workers to stop
	close(ps.queue)

	// Wait for all workers to finish
	ps.wg.Wait()

	ps.isRunning = false
	logging.Infof("Push notification service stopped")
}

// ProcessEvent processes a Nostr event and determines if push notifications should be sent
func (ps *PushService) ProcessEvent(event *nostr.Event) {
	if ps == nil || !ps.isRunning {
		return
	}

	// Log incoming event for push notification processing
	logging.Infof("🔔 Processing event for push notifications - Kind: %d, Event ID: %s, Author: %s",
		event.Kind, event.ID, event.PubKey)

	// Check if this event type should trigger notifications
	if !ps.shouldNotify(event) {
		logging.Debugf("Event kind %d does not trigger notifications", event.Kind)
		return
	}

	// Get users who should be notified for this event
	recipients := ps.getNotificationRecipients(event)
	logging.Infof("📬 Found %d recipients for notification (Kind: %d)", len(recipients), event.Kind)

	for _, pubkey := range recipients {
		// Avoid notifying yourself
		if pubkey == event.PubKey {
			continue
		}

		// Get devices for this user
		// Need to get StatsStore first
		statsStore := ps.store.GetStatsStore()
		if statsStore == nil {
			logging.Errorf("Stats store not available")
			continue
		}

		devices, err := statsStore.GetPushDevicesByPubkey(pubkey)
		if err != nil {
			logging.Errorf("Failed to get devices for pubkey %s: %v", pubkey, err)
			continue
		}

		// Format notification message
		message := ps.formatNotificationMessage(event, pubkey)

		// Queue notification for each device
		for _, device := range devices {
			task := &NotificationTask{
				Pubkey:      pubkey,
				Event:       event,
				DeviceToken: device.DeviceToken,
				Platform:    device.Platform,
				Message:     message,
				Attempts:    0,
			}

			select {
			case ps.queue <- task:
				// Successfully queued
				logging.Infof("✉️ Queued push notification for %s on %s (Queue size: %d/%d)",
					pubkey[:8], device.Platform, len(ps.queue), cap(ps.queue))
				logging.Infof("✉️ Notification details - Event: %s, Kind: %d, Title: %s",
					event.ID, event.Kind, message.Title)
			case <-ps.ctx.Done():
				logging.Infof("⚠️ Push service shutting down, notification not queued")
				return
			default:
				logging.Warnf("⚠️ Push notification queue is full (%d/%d), dropping notification for %s",
					cap(ps.queue), cap(ps.queue), pubkey)
			}
		}
	}
}

// shouldNotify determines if an event should trigger push notifications
func (ps *PushService) shouldNotify(event *nostr.Event) bool {
	// Based on the plan, we focus on specific event kinds
	switch event.Kind {
	case 1: // Text note reply
		logging.Infof("✅ Event kind 1 (Text Note Reply) will trigger notifications")
		return true
	case 1808: // Audio notes (mentions and replies)
		logging.Infof("✅ Event kind 1808 (Audio note) will trigger notifications")
		return true
	case 1809: // Audio post repost
		logging.Infof("✅ Event kind 1809 (Audio post repost) will trigger notifications")
		return true
	case 3: // Contact lists (new followers)
		logging.Infof("✅ Event kind 3 (Contact list) will trigger notifications")
		return true
	case 4: // DMs
		logging.Infof("✅ Event kind 4 (DM) will trigger notifications")
		return true
	case 6: // Reposts
		logging.Infof("✅ Event kind 6 (Repost) will trigger notifications")
		return true
	case 7: // Reactions
		if event.Content == "-" {
			return false
		}
		logging.Infof("✅ Event kind 7 (Reaction) will trigger notifications")
		return true
	case 1059: // Gift Wrap (NIP-59 encrypted DMs)
		logging.Infof("✅ Event kind 1059 (Gift Wrap DM) will trigger notifications")
		return true
	default:
		return false
	}
}

// getNotificationRecipients determines which users should receive notifications for an event
func (ps *PushService) getNotificationRecipients(event *nostr.Event) []string {
	var recipients []string
	recipientsMap := make(map[string]bool) // Use map to avoid duplicates

	// Helper to add recipient
	addRecipient := func(pubkey string) {
		if pubkey != "" && !recipientsMap[pubkey] {
			recipientsMap[pubkey] = true
			recipients = append(recipients, pubkey)
		}
	}

	switch event.Kind {
	case 1: // Text Note Reply
		// 1. Notify p-tags (Mentions)
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				addRecipient(tag[1])
				logging.Infof("👤 Added recipient for mention (p-tag): %s", tag[1])
			}
		}
		// 2. Notify author of parent event (Reply)
		if author := ps.getAuthorOfRefEvent(event); author != "" {
			addRecipient(author)
			logging.Infof("👤 Added recipient for reply (parent author): %s", author)
		}

	case 1808: // Audio notes
		// 1. Notify mentions in p tags
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				addRecipient(tag[1])
				logging.Infof("👤 Added recipient for audio note mention: %s", tag[1])
			}
		}
		// 2. Notify author of parent event (Reply)
		if author := ps.getAuthorOfRefEvent(event); author != "" {
			addRecipient(author)
			logging.Infof("👤 Added recipient for audio reply (parent author): %s", author)
		}

	case 6, 1809: // Repost / Audio Repost
		// Notify the original author
		if author := ps.getAuthorOfRefEvent(event); author != "" {
			addRecipient(author)
			logging.Infof("👤 Added recipient for repost (original author): %s", author)
		}
		// Also notify p-tags just in case
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				addRecipient(tag[1])
			}
		}

	case 7: // Reactions
		// Notify the author of the reacted-to event
		if author := ps.getAuthorOfRefEvent(event); author != "" {
			addRecipient(author)
			logging.Infof("👤 Added recipient for reaction (original author): %s", author)
		}

	case 3: // Contact list - notify the users being followed
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				addRecipient(tag[1])
				logging.Infof("👤 Added recipient for new follower: %s", tag[1])
			}
		}

	case 4: // DM - notify the recipient
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				addRecipient(tag[1])
				logging.Infof("👤 Added recipient for DM: %s", tag[1])
			}
		}

	case 1059: // Gift Wrap (NIP-59 encrypted DM) - notify the recipient
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				addRecipient(tag[1])
				logging.Infof("👤 Added recipient for Gift Wrap DM: %s", tag[1])
			}
		}
	}

	return recipients
}

// getAuthorOfRefEvent looks up the author of the event referenced by an 'e' tag.
// It prioritizes the "reply" marker, then the root/last convention, or just the first e-tag.
func (ps *PushService) getAuthorOfRefEvent(event *nostr.Event) string {
	var refEventID string

	// 1. Look for 'e' tag with marker 'reply'
	for _, tag := range event.Tags {
		if len(tag) >= 4 && tag[0] == "e" && tag[3] == "reply" {
			refEventID = tag[1]
			break
		}
	}

	// 2. If no reply marker, look for the last 'e' tag (NIP-10 mostly)
	if refEventID == "" {
		for i := len(event.Tags) - 1; i >= 0; i-- {
			tag := event.Tags[i]
			if len(tag) >= 2 && tag[0] == "e" {
				refEventID = tag[1]
				break
			}
		}
	}

	if refEventID == "" {
		return ""
	}

	// Query the store for the Referenced Event
	filter := nostr.Filter{
		IDs:   []string{refEventID},
		Limit: 1,
	}

	events, err := ps.store.QueryEvents(filter)
	if err != nil {
		logging.Errorf("Failed to query referenced event %s: %v", refEventID, err)
		return ""
	}

	if len(events) > 0 {
		return events[0].PubKey
	}

	logging.Warnf("Referenced event %s not found in store", refEventID)
	return ""
}

// getAuthorName looks up the profile of the event author to get their name
func (ps *PushService) getAuthorName(pubkey string) string {
	// Special case for test notifications
	if pubkey == "0000000000000000000000000000000000000000000000000000000000000000" {
		return "Test Notification"
	}

	// Check cache first
	ps.cacheMutex.RLock()
	if name, found := ps.nameCache[pubkey]; found {
		ps.cacheMutex.RUnlock()
		logging.Infof("✅ Cache hit for %s: %s", shortenPubkey(pubkey), name)
		return name
	}
	ps.cacheMutex.RUnlock()

	// Query the store for the author's Kind 0 (Metadata) event
	filter := nostr.Filter{
		Kinds:   []int{0},
		Authors: []string{pubkey},
		Limit:   1,
	}

	logging.Infof("🔍 Looking up profile for pubkey: %s", shortenPubkey(pubkey))

	events, err := ps.store.QueryEvents(filter)
	if err != nil {
		logging.Warnf("Failed to query profile for %s: %v", pubkey, err)
		return shortenPubkey(pubkey)
	}

	if len(events) == 0 {
		logging.Infof("No kind 0 profile found for %s", shortenPubkey(pubkey))

		// Cache the shortened pubkey even if no profile exists
		shortened := shortenPubkey(pubkey)
		ps.cacheMutex.Lock()
		ps.nameCache[pubkey] = shortened
		ps.cacheMutex.Unlock()

		return shortened
	}

	logging.Infof("Found kind 0 profile for %s, content preview: %s",
		shortenPubkey(pubkey),
		func() string {
			if len(events[0].Content) > 100 {
				return events[0].Content[:100] + "..."
			}
			return events[0].Content
		}())

	// Parse the content
	var profile map[string]interface{}
	if err := json.Unmarshal([]byte(events[0].Content), &profile); err != nil {
		logging.Warnf("Failed to parse profile content for %s: %v", pubkey, err)
		return shortenPubkey(pubkey)
	}

	// Try multiple field name variations that are commonly used
	nameFields := []string{"display_name", "displayName", "name", "username", "handle"}

	for _, field := range nameFields {
		if value, ok := profile[field]; ok {
			if name, ok := value.(string); ok && name != "" {
				logging.Infof("✅ Found name for %s: %s (from field: %s)",
					shortenPubkey(pubkey), name, field)

				// Cache the name
				ps.cacheMutex.Lock()
				ps.nameCache[pubkey] = name
				ps.cacheMutex.Unlock()

				return name
			}
		}
	}

	// If we have profile data but no name, log what fields are available
	if len(profile) > 0 {
		fields := make([]string, 0, len(profile))
		for k := range profile {
			fields = append(fields, k)
		}
		logging.Infof("Profile for %s has fields: %v but no recognized name field",
			shortenPubkey(pubkey), fields)
	}

	logging.Infof("No name found in profile for %s, using shortened pubkey", shortenPubkey(pubkey))

	// Cache the shortened pubkey to avoid repeated lookups
	shortened := shortenPubkey(pubkey)
	ps.cacheMutex.Lock()
	ps.nameCache[pubkey] = shortened
	ps.cacheMutex.Unlock()

	return shortened
}

func shortenPubkey(pubkey string) string {
	if len(pubkey) < 8 {
		return pubkey
	}
	return fmt.Sprintf("%s...%s", pubkey[:4], pubkey[len(pubkey)-4:])
}

// formatNotificationMessage formats a push notification message for an event
func (ps *PushService) formatNotificationMessage(event *nostr.Event, recipient string) *PushMessage {
	message := &PushMessage{
		Badge:    1,
		Sound:    "default",
		Category: fmt.Sprintf("kind_%d", event.Kind),
		Data: map[string]interface{}{
			"event_id":   event.ID,
			"event_kind": event.Kind,
			"pubkey":     event.PubKey,
		},
	}

	authorName := ps.getAuthorName(event.PubKey)

	switch event.Kind {
	case 1: // Text Note
		message.Title = "New Reply"
		message.Body = fmt.Sprintf("%s replied to your note", authorName)

	case 1808: // Audio note
		message.Title = "New Audio Note"
		message.Body = fmt.Sprintf("%s mentioned you in an audio note", authorName)

	case 1809: // Audio post repost
		message.Title = "Audio Repost"
		message.Body = fmt.Sprintf("%s reposted your audio post", authorName)

	case 3: // Contact list
		message.Title = "New Follower"
		message.Body = fmt.Sprintf("%s started following you", authorName)

	case 4: // DM
		message.Title = "New Message"
		message.Body = fmt.Sprintf("You have a new direct message from %s", authorName)

	case 6: // Repost
		message.Title = "Repost"
		message.Body = fmt.Sprintf("%s reposted your note", authorName)

	case 7: // Reaction
		// Check content for emoji to make notification nicer
		var content string
		switch event.Content {
		case "", "+":
			content = "liked"
		default:
			// If it's a longer reaction (emoji), show it
			content = fmt.Sprintf("reacted %s to", event.Content)
		}
		message.Title = "New Reaction"
		message.Body = fmt.Sprintf("%s %s your note", authorName, content)

	case 1059: // Gift Wrap (NIP-59 encrypted DM)
		// For gift wraps, we can't show the real sender as it's encrypted
		// The pubkey is a random ephemeral key
		message.Title = "New Encrypted Message"
		message.Body = "You have a new encrypted message"

	default:
		message.Title = "New Notification"
		message.Body = "You have a new notification"
	}

	logging.Infof("📱 Formatted notification - Title: %s, Body: %s, Recipient: %s",
		message.Title, message.Body, recipient)

	return message
}

// Global service instance
var globalPushService *PushService
var serviceMutex sync.RWMutex

// InitGlobalPushService initializes the global push service instance
func InitGlobalPushService(store stores.Store) error {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()

	if globalPushService != nil {
		return fmt.Errorf("push service already initialized")
	}

	service, err := NewPushService(store)
	if err != nil {
		return err
	}

	if service != nil {
		if err := service.Start(); err != nil {
			return err
		}
	}

	globalPushService = service
	return nil
}

// GetGlobalPushService returns the global push service instance
func GetGlobalPushService() *PushService {
	serviceMutex.RLock()
	defer serviceMutex.RUnlock()
	return globalPushService
}

// StopGlobalPushService stops the global push service
func StopGlobalPushService() {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()

	if globalPushService != nil {
		globalPushService.Stop()
		globalPushService = nil
	}
}

// ReloadGlobalPushService reloads the global push service with updated configuration
func ReloadGlobalPushService(store stores.Store) error {
	serviceMutex.Lock()
	defer serviceMutex.Unlock()

	// Stop existing service if running
	if globalPushService != nil {
		logging.Infof("Stopping existing push service for reload...")
		globalPushService.Stop()
		globalPushService = nil
	}

	// Create new service with updated configuration
	service, err := NewPushService(store)
	if err != nil {
		return fmt.Errorf("failed to create new push service: %w", err)
	}

	// Start the new service if it was created (might be nil if disabled)
	if service != nil {
		if err := service.Start(); err != nil {
			return fmt.Errorf("failed to start new push service: %w", err)
		}
		logging.Infof("Push service reloaded successfully")
	} else {
		logging.Infof("Push service is disabled in configuration")
	}

	globalPushService = service
	return nil
}
