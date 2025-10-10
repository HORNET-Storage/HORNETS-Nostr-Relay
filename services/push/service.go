package push

import (
	"context"
	"fmt"
	"sync"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/nbd-wtf/go-nostr"
)

// PushService manages push notifications
type PushService struct {
	store      statistics.StatisticsStore
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
func NewPushService(store statistics.StatisticsStore) (*PushService, error) {
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
		store:  store,
		config: &cfg.PushNotifications,
		queue:  make(chan *NotificationTask, cfg.PushNotifications.Service.QueueSize),
		ctx:    ctx,
		cancel: cancel,
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
		// Get devices for this user
		devices, err := ps.store.GetPushDevicesByPubkey(pubkey)
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
		logging.Infof("✅ Event kind 7 (Reaction) will trigger notifications")
		return true
	default:
		return false
	}
}

// getNotificationRecipients determines which users should receive notifications for an event
func (ps *PushService) getNotificationRecipients(event *nostr.Event) []string {
	var recipients []string

	switch event.Kind {
	case 1808: // Audio notes
		// Check for mentions in p tags
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				recipients = append(recipients, tag[1])
				logging.Infof("👤 Added recipient for audio note mention: %s", tag[1])
			}
		}

	case 1809: // Audio post repost
		// Notify the original author from p tags
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				recipients = append(recipients, tag[1])
				logging.Infof("👤 Added recipient for audio repost: %s", tag[1])
			}
		}

	case 3: // Contact list - notify the users being followed
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				recipients = append(recipients, tag[1])
				logging.Infof("👤 Added recipient for new follower: %s", tag[1])
			}
		}

	case 4: // DM - notify the recipient
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "p" {
				recipients = append(recipients, tag[1])
				logging.Infof("👤 Added recipient for DM: %s", tag[1])
			}
		}

	case 6, 7: // Reposts and reactions - notify the author of the referenced event
		for _, tag := range event.Tags {
			if len(tag) >= 2 && tag[0] == "e" {
				// We would need to look up the author of the referenced event
				// For now, skip this complex lookup
				logging.Debugf("⚠️ Event reference found but author lookup not implemented: %s", tag[1])
			}
		}
	}

	return recipients
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

	switch event.Kind {
	case 1808: // Audio note
		message.Title = "New Audio Note"
		message.Body = "You were mentioned in an audio note"

	case 1809: // Audio post repost
		message.Title = "Audio Repost"
		message.Body = "Someone reposted your audio post"

	case 3: // Contact list
		message.Title = "New Follower"
		message.Body = "Someone started following you"

	case 4: // DM
		message.Title = "New Message"
		message.Body = "You have a new direct message"

	case 6: // Repost
		message.Title = "Repost"
		message.Body = "Someone reposted your note"

	case 7: // Reaction
		message.Title = "Reaction"
		message.Body = "Someone reacted to your note"

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
func InitGlobalPushService(store statistics.StatisticsStore) error {
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
func ReloadGlobalPushService(store statistics.StatisticsStore) error {
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
