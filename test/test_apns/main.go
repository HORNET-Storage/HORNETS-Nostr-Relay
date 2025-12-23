package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/services/push"
)

func main() {
	// Parse command line arguments
	deviceToken := flag.String("token", "", "The device token to send the notification to")
	flag.Parse()

	if *deviceToken == "" {
		fmt.Println("Usage: go run tools/test_apns.go -token <DEVICE_TOKEN>")
		os.Exit(1)
	}

	// Initialize configuration
	if err := config.InitConfig(); err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// Get configuration
	cfg, err := config.GetConfig()
	if err != nil {
		log.Fatalf("Failed to get config: %v", err)
	}

	// Check if APNS is enabled
	if !cfg.PushNotifications.APNS.Enabled {
		log.Println("⚠️ APNs is NOT enabled in config.yaml")
		log.Println("Please set 'push_notifications.apns.enabled: true' in your config.yaml")
		// Continue anyway to try and test with what's there, but warn
	}

	fmt.Println("🔧 Testing APNs Configuration...")
	fmt.Printf("Key Path: %s\n", cfg.PushNotifications.APNS.KeyPath)
	fmt.Printf("Key ID: %s\n", cfg.PushNotifications.APNS.KeyID)
	fmt.Printf("Team ID: %s\n", cfg.PushNotifications.APNS.TeamID)
	fmt.Printf("Bundle ID: %s\n", cfg.PushNotifications.APNS.BundleID)
	fmt.Printf("Production: %v\n", cfg.PushNotifications.APNS.Production)

	// Create APNS client directly
	client, err := push.NewAPNSClient(&cfg.PushNotifications.APNS)
	if err != nil {
		log.Fatalf("❌ Failed to create APNs client: %v", err)
	}

	// Create a test message
	message := &push.PushMessage{
		Title:    "Test Notification",
		Body:     "This is a test notification from the Hornets Relay CLI tool.",
		Badge:    1,
		Sound:    "default",
		Category: "TEST_CATEGORY",
		Data: map[string]interface{}{
			"test_id":   "12345",
			"timestamp": "now",
		},
	}

	fmt.Printf("\n🚀 Sending test notification to token: %s...\n", *deviceToken)

	// Send notification
	err = client.SendNotification(*deviceToken, message)
	if err != nil {
		log.Fatalf("❌ Failed to send notification: %v", err)
	}

	fmt.Println("✅ Notification sent successfully!")
}
