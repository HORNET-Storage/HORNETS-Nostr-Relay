package push

import (
	"encoding/json"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// NewFCMClient creates a new FCM client (simplified mock implementation for now)
func NewFCMClient(config *types.FCMConfig) (FCMClient, error) {
	// For now, return mock client until we add proper FCM dependencies
	logging.Infof("Creating FCM client (mock mode) for project ID: %s", config.ProjectID)
	return &MockFCMClient{projectID: config.ProjectID}, nil
}

// MockFCMClient is a mock implementation for testing
type MockFCMClient struct {
	projectID string
}

// SendNotification mock implementation
func (m *MockFCMClient) SendNotification(deviceToken string, message *PushMessage) error {
	// Simulate sending notification
	// Show partial token for privacy
	tokenDisplay := deviceToken
	if len(deviceToken) > 16 {
		tokenDisplay = deviceToken[:8] + "..." + deviceToken[len(deviceToken)-8:]
	}

	logging.Warn("🤖 Mock FCM Notification (NOT SENT)", map[string]interface{}{
		"device_token": tokenDisplay,
		"title":        message.Title,
		"body":         message.Body,
		"note":         "This is a MOCK - no actual notification was sent to the device",
		"help":         "To enable real notifications, configure FCM credentials in config.yaml",
	})

	// Log the full message for debugging
	messageJSON, _ := json.MarshalIndent(message, "", "  ")
	logging.Debugf("Mock FCM full payload: %s", string(messageJSON))

	return nil
}
