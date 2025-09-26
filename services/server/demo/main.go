package main

import (
	"fmt"
	"os"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/sqlite"
	"github.com/HORNET-Storage/hornet-storage/lib/web"
	"github.com/HORNET-Storage/hornet-storage/services/server/demo/demodata"
	"github.com/spf13/viper"
)

func init() {
	// Initialze config system
	err := config.InitConfig()
	if err != nil {
		logging.Fatalf("Failed to initialize config: %v", err)
	}

	// Initialize logging system
	if err := logging.InitLogger(); err != nil {
		logging.Fatalf("Failed to initialize logger: %v", err)
	}

	viper.Set("server.demo", true)
}

func main() {
	logging.Info("========================================")
	logging.Info("  HORNETS RELAY DEMO MODE")
	logging.Info("  Authentication bypassed for admin panel")
	logging.Info("  For demonstration purposes only")
	logging.Info("  NOT FOR PRODUCTION USE")
	logging.Info("========================================")

	// Use a separate data directory for the demo server to avoid conflicts
	// Initialize BadgerHold store with a separate data directory for demo mode and custom statistics DB path
	dbPath := "./data/store"
	store, err := badgerhold.InitStore(dbPath)
	if err != nil {
		logging.Fatalf("Failed to initialize BadgerHold store: %v", err)
	}

	defer store.Cleanup()

	// Check if the database is empty and generate demo data if needed
	isEmpty, count, err := databaseIsEmptyWithCount(dbPath)
	if err != nil {
		logging.Infof("Warning: Failed to check if database is empty: %v", err)
	} else if isEmpty {
		logging.Info("Statistics database is empty, generating demo data...")
		if err := generateDemoData(store); err != nil {
			logging.Infof("Warning: Failed to generate demo data: %v", err)
			logging.Info("Demo charts may not display correctly without data")
		} else {
			logging.Info("Successfully generated demo data for statistics visualization")
		}
	} else {
		logging.Infof("Using existing demo data in statistics database (found %d events)", count)
	}

	// Set up cleanup on exit
	defer func() {
		logging.Info("Cleaning up demo relay resources...")
		err := store.Cleanup()
		if err != nil {
			logging.Infof("Failed to cleanup demo data: %v", err)
		} else {
			logging.Info("Demo data cleanup successful")
		}
	}()
	// Log which ports will be used
	demoPort := config.GetPort("web")
	if demoPort > 0 {
		logging.Infof("Demo server will use port %d (web panel on port %d)", demoPort-2, demoPort)
	}

	logging.Info("Starting demo web server...")
	err = web.StartServer(store)

	if err != nil {
		logging.Fatalf("Fatal error occurred in demo web server: %v", err)
	}
}

// databaseIsEmptyWithCount checks if the statistics database is empty and returns the count
func databaseIsEmptyWithCount(dbPath string) (bool, int, error) {
	// If the database file doesn't exist, it's empty
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return true, 0, nil
	}

	// Initialize the database connection with the specified path
	store, err := sqlite.InitStore(dbPath)
	if err != nil {
		return true, 0, fmt.Errorf("error connecting to SQLite database: %v", err)
	}

	// Check if there are any event kinds in the database
	count, err := store.FetchKindCount()
	if err != nil {
		return true, 0, fmt.Errorf("error checking event kinds: %v", err)
	}

	// If there are no events, the database is considered empty
	return count == 0, count, nil
}

// generateDemoData creates demo data in the statistics database
func generateDemoData(store *badgerhold.BadgerholdStore) error {
	// Create a new generator with default settings
	generator := demodata.NewDemoDataGenerator()

	// Generate all types of demo data
	if err := generator.GenerateAllData(store); err != nil {
		return fmt.Errorf("error generating demo data: %v", err)
	}

	return nil
}
