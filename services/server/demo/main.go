package main

import (
	"fmt"
	"log"
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
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// Initialize logging system
	if err := logging.InitLogger(); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	viper.Set("server.demo", true)
}

func main() {
	log.Println("========================================")
	log.Println("  HORNETS RELAY DEMO MODE")
	log.Println("  Authentication bypassed for admin panel")
	log.Println("  For demonstration purposes only")
	log.Println("  NOT FOR PRODUCTION USE")
	log.Println("========================================")

	// Use a separate data directory for the demo server to avoid conflicts
	// Initialize BadgerHold store with a separate data directory for demo mode
	store, err := badgerhold.InitStore("demo-data")
	if err != nil {
		log.Fatal(err)
	}

	// Switch to using a separate statistics database for demo mode
	// This ensures we don't mix demo data with production statistics
	dbPath := "demo_statistics.db"
	if err := store.UseDemoStatisticsDB(); err != nil {
		log.Printf("Warning: Failed to switch to demo statistics database: %v", err)
		log.Println("Continuing with standard statistics database...")
		// Continue anyway as this is not a critical failure
	} else {
		log.Println("Demo server is using a separate statistics database (demo_statistics.db)")

		// Check if the database is empty and generate demo data if needed
		isEmpty, count, err := databaseIsEmptyWithCount(dbPath)
		if err != nil {
			log.Printf("Warning: Failed to check if database is empty: %v", err)
		} else if isEmpty {
			log.Println("Statistics database is empty, generating demo data...")
			if err := generateDemoData(dbPath); err != nil {
				log.Printf("Warning: Failed to generate demo data: %v", err)
				log.Println("Demo charts may not display correctly without data")
			} else {
				log.Println("Successfully generated demo data for statistics visualization")
			}
		} else {
			log.Printf("Using existing demo data in statistics database (found %d events)", count)
		}
	}

	// Set up cleanup on exit
	defer func() {
		log.Println("Cleaning up demo relay resources...")
		err := store.Cleanup()
		if err != nil {
			log.Printf("Failed to cleanup demo data: %v", err)
		} else {
			log.Println("Demo data cleanup successful")
		}
	}()
	// Log which ports will be used
	demoPort := config.GetPort("web")
	if demoPort > 0 {
		log.Printf("Demo server will use port %d (web panel on port %d)", demoPort-2, demoPort)
	}

	log.Println("Starting demo web server...")
	err = web.StartServer(store)

	if err != nil {
		log.Fatalf("Fatal error occurred in demo web server: %v", err)
	}
}

// databaseIsEmptyWithCount checks if the statistics database is empty and returns the count
func databaseIsEmptyWithCount(dbPath string) (bool, int, error) {
	// If the database file doesn't exist, it's empty
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return true, 0, nil
	}

	// Initialize the database connection
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
func generateDemoData(dbPath string) error {
	// Create a new generator with default settings
	generator := demodata.NewDemoDataGenerator()

	// Initialize the SQLite store
	store, err := sqlite.InitStore(dbPath)
	if err != nil {
		return fmt.Errorf("error initializing SQLite store: %v", err)
	}

	// Generate all types of demo data
	if err := generator.GenerateAllData(store); err != nil {
		return fmt.Errorf("error generating demo data: %v", err)
	}

	return nil
}
