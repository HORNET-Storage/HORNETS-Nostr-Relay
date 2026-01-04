package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	// Initialize configuration to get data path
	if err := config.InitConfig(); err != nil {
		log.Printf("Warning: Failed to initialize config: %v", err)
	}

	dataDir := config.GetDataDir()

	// Check for potential database files
	possibleDBs := []string{
		filepath.Join(dataDir, "statistics", "statistics.db"), // Real database path
		filepath.Join(dataDir, "hornet-storage.db"),
		filepath.Join(dataDir, "demo_statistics.db"),
	}

	var dbPath string
	for _, path := range possibleDBs {
		if _, err := os.Stat(path); err == nil {
			dbPath = path
			break
		}
	}

	if dbPath == "" {
		// Try searching in current directory
		if _, err := os.Stat("statistics.db"); err == nil {
			dbPath = "statistics.db"
		} else if _, err := os.Stat("hornet-storage.db"); err == nil {
			dbPath = "hornet-storage.db"
		} else if _, err := os.Stat("demo_statistics.db"); err == nil {
			dbPath = "demo_statistics.db"
		}
	}

	if dbPath == "" {
		log.Fatalf("❌ Could not find database file in %s or current directory", dataDir)
	}

	fmt.Printf("📂 Using database: %s\n", dbPath)

	// Open database
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalf("❌ Failed to open database: %v", err)
	}

	// Query devices
	var devices []types.PushDevice
	if err := db.Order("created_at desc").Find(&devices).Error; err != nil {
		log.Fatalf("❌ Failed to query devices: %v", err)
	}

	if len(devices) == 0 {
		fmt.Println("⚠️ No devices found in the database. Ensure your device has connected and registered.")
		return
	}

	fmt.Printf("\n📱 Found %d registered devices:\n", len(devices))
	fmt.Println("--------------------------------------------------")

	for _, device := range devices {
		status := "Active"
		if !device.IsActive {
			status = "Inactive"
		}

		fmt.Printf("ID: %d\n", device.ID)
		fmt.Printf("Pubkey: %s\n", device.Pubkey)
		fmt.Printf("Platform: %s\n", device.Platform)
		fmt.Printf("Token: %s\n", device.DeviceToken)
		fmt.Printf("Status: %s\n", status)
		fmt.Printf("Last Updated: %s\n", device.UpdatedAt.Format(time.RFC822))
		fmt.Println("--------------------------------------------------")
	}

	fmt.Println("\nTo test a notification, copy a token and run:")
	fmt.Println("go run tools/test_apns.go -token <TOKEN>")
}
