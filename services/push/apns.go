package push

import (
	"encoding/json"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// NewAPNSClient creates a new APNs client (simplified mock implementation for now)
func NewAPNSClient(config *types.APNSConfig) (APNSClient, error) {
	// For now, return mock client until we add proper APNs dependencies
	logging.Infof("Creating APNs client (mock mode) for bundle ID: %s", config.BundleID)
	return &MockAPNSClient{bundleID: config.BundleID}, nil
}

// MockAPNSClient is a mock implementation for testing
type MockAPNSClient struct {
	bundleID string
}

// SendNotification mock implementation
func (m *MockAPNSClient) SendNotification(deviceToken string, message *PushMessage) error {
	// Simulate sending notification
	logging.Infof("Mock APNs: Would send notification to %s: %s - %s", deviceToken, message.Title, message.Body)

	// Log the full message for debugging
	messageJSON, _ := json.MarshalIndent(message, "", "  ")
	logging.Debugf("Mock APNs message payload: %s", string(messageJSON))

	return nil
}
