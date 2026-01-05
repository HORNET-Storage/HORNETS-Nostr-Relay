package push

import (
	"fmt"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/token"
)

// NewAPNSClient creates a new APNs client
func NewAPNSClient(config *types.APNSConfig) (APNSClient, error) {
	// Check if we have the necessary configuration for real APNs
	if config.KeyPath == "" || config.KeyID == "" || config.TeamID == "" {
		logging.Warn("⚠️ APNs Configuration Missing - Using Mock Client", map[string]interface{}{
			"message":   "Real APNs requires KeyPath, KeyID, and TeamID",
			"bundle_id": config.BundleID,
		})
		return &MockAPNSClient{
			bundleID:   config.BundleID,
			production: config.Production,
		}, nil
	}

	authKey, err := token.AuthKeyFromFile(config.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load APNs auth key from %s: %w", config.KeyPath, err)
	}

	token := &token.Token{
		AuthKey: authKey,
		KeyID:   config.KeyID,
		TeamID:  config.TeamID,
	}

	client := apns2.NewTokenClient(token)
	if config.Production {
		client = client.Production()
	} else {
		client = client.Development()
	}

	logging.Infof("✅ APNs Client Initialized (Production: %v)", config.Production)

	return &RealAPNSClient{
		client:     client,
		bundleID:   config.BundleID,
		production: config.Production,
	}, nil
}

// RealAPNSClient is the real implementation using Apple Push Notification service
type RealAPNSClient struct {
	client     *apns2.Client
	bundleID   string
	production bool
}

// SendNotification sends a push notification to an iOS device
func (c *RealAPNSClient) SendNotification(deviceToken string, message *PushMessage) error {
	payload := message.ToAPNsPayload()

	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       c.bundleID,
		Payload:     payload,
	}

	res, err := c.client.Push(notification)
	if err != nil {
		logging.Errorf("Failed to send APNs notification: %v", err)
		return err
	}

	if res.Sent() {
		logging.Infof("🚀 APNs Notification Sent: %v %v %v", res.StatusCode, res.ApnsID, res.Reason)
	} else {
		logging.Warn("⚠️ APNs Notification Failed", map[string]interface{}{
			"status":  res.StatusCode,
			"reason":  res.Reason,
			"apns_id": res.ApnsID,
			"token":   deviceToken,
		})
		return fmt.Errorf("APNs notification failed: %v %v", res.StatusCode, res.Reason)
	}

	return nil
}

// MockAPNSClient is a mock implementation for testing
type MockAPNSClient struct {
	bundleID   string
	production bool
}

// SendNotification mock implementation
func (m *MockAPNSClient) SendNotification(deviceToken string, message *PushMessage) error {
	// Simulate sending notification
	environment := "development"
	if m.production {
		environment = "production"
	}

	// Show partial token for privacy
	tokenDisplay := deviceToken
	if len(deviceToken) > 16 {
		tokenDisplay = deviceToken[:8] + "..." + deviceToken[len(deviceToken)-8:]
	}

	logging.Warn("📱 Mock APNs Notification (NOT SENT)", map[string]interface{}{
		"device_token":    tokenDisplay,
		"bundle_id":       m.bundleID,
		"environment":     environment,
		"title":           message.Title,
		"body":            message.Body,
		"mutable_content": message.MutableContent,
		"note":            "This is a MOCK - no actual notification was sent to the device",
		"help":            "For local dev: Use TestFlight or configure real APNs credentials",
	})

	return nil
}

// Helper to convert PushMessage to APNs payload
func (m *PushMessage) ToAPNsPayload() map[string]interface{} {
	aps := map[string]interface{}{
		"alert": map[string]interface{}{
			"title": m.Title,
			"body":  m.Body,
		},
		"badge": m.Badge,
		"sound": m.Sound,
	}

	// Add mutable-content flag for iOS to allow notification modification
	if m.MutableContent {
		aps["mutable-content"] = 1
	}

	if m.Category != "" {
		aps["category"] = m.Category
	}

	payload := map[string]interface{}{
		"aps": aps,
	}

	// Add custom data
	for k, v := range m.Data {
		payload[k] = v
	}

	return payload
}
