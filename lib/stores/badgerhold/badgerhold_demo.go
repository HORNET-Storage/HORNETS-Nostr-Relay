package badgerhold

import (
	"fmt"
	"log"

	// Import the demo SQLite package we just created
	statistics_gorm_sqlite_demo "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/sqlite_demo"
)

// UseDemoStatisticsDB replaces the standard statistics database with a demo version
// This function should be called after the BadgerholdStore is initialized
// in the demo server to use a separate statistics database for demonstration purposes
func (store *BadgerholdStore) UseDemoStatisticsDB() error {
	var err error

	// Initialize the demo statistics database
	log.Println("Switching to demo statistics database...")

	// Initialize demo statistics database
	// This will create a new database file called demo_statistics.db
	demoStatsDB, err := statistics_gorm_sqlite_demo.InitStore()
	if err != nil {
		return fmt.Errorf("failed to initialize demo statistics database: %v", err)
	}

	// Replace the existing statistics database connection with our demo version
	store.StatsDatabase = demoStatsDB

	log.Println("Successfully switched to demo statistics database")
	return nil
}
