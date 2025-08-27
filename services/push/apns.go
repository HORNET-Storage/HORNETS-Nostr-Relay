package push

import (
	"encoding/json"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// NewAPNSClient creates a new APNs client (simplified mock implementation for now)
func NewAPNSClient(config *types.APNSConfig) (APNSClient, error) {
	// For now, return mock client until we add proper APNs dependencies
	logging.Warn("⚠️ APNs Mock Mode Active", map[string]interface{}{
		"message":   "Using mock APNs client - notifications will NOT be delivered to devices",
		"bundle_id": config.BundleID,
		"reason":    "Real APNs implementation requires certificate configuration",
		"solution":  "See docs/APNS_CONFIGURATION_GUIDE.md for setup instructions",
	})
	return &MockAPNSClient{
		bundleID:   config.BundleID,
		production: config.Production,
	}, nil
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
		"device_token": tokenDisplay,
		"bundle_id":    m.bundleID,
		"environment":  environment,
		"title":        message.Title,
		"body":         message.Body,
		"note":         "This is a MOCK - no actual notification was sent to the device",
		"help":         "For local dev: Use TestFlight or configure real APNs credentials",
	})

	// Log the full message for debugging
	messageJSON, _ := json.MarshalIndent(message, "", "  ")
	logging.Debugf("Mock APNs full payload: %s", string(messageJSON))

	// Common environment mismatch hints
	if len(deviceToken) != 64 {
		logging.Warn("Invalid device token length", map[string]interface{}{
			"expected": 64,
			"actual":   len(deviceToken),
			"token":    deviceToken,
		})
	}

	return nil
}
