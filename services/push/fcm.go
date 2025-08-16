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
	logging.Infof("Mock FCM: Would send notification to %s: %s - %s", deviceToken, message.Title, message.Body)

	// Log the full message for debugging
	messageJSON, _ := json.MarshalIndent(message, "", "  ")
	logging.Debugf("Mock FCM message payload: %s", string(messageJSON))

	return nil
}
