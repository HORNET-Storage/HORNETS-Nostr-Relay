package sqlite

import (
	"fmt"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	statistics_gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm"
)

func InitStore(args ...interface{}) (*statistics_gorm.GormStatisticsStore, error) {
	store := &statistics_gorm.GormStatisticsStore{}

	var err error
	
	// Configure SQLite with proper connection handling:
	// - journal_mode=WAL enables Write-Ahead Logging for better concurrency
	// - busy_timeout=10000 waits up to 10 seconds when database is locked
	// - _txlock=immediate begins transactions sooner to reduce deadlocks
	// - _synchronous=normal provides a balance of safety and performance
	dsn := "statistics.db?_journal_mode=WAL&_busy_timeout=10000&_txlock=immediate&_synchronous=normal&cache=shared"
	
	// Configure GORM with more advanced settings
	store.DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// Logger configuration to show SQL queries during development
		Logger: gorm.Default.LogMode(gorm.Silent), // Change to Info for debugging
		
		// Performance improvements
		PrepareStmt: true, // Caches prepared statements for better performance
		
		// Avoid global lock issues in SQLite
		DisableAutomaticPing: true,
		
		// Set a more lenient query timeout
		NowFunc: func() time.Time {
			return time.Now()
		},
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SQLite database: %v", err)
	}

	// Configure connection pool settings
	sqlDB, err := store.DB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %v", err)
	}
	
	// Set connection pool parameters
	sqlDB.SetMaxIdleConns(5)     // Maximum number of idle connections
	sqlDB.SetMaxOpenConns(20)    // Maximum number of open connections
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // Maximum connection lifetime
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Maximum idle connection lifetime

	// Initialize store schema
	err = store.Init()
	if err != nil {
		return nil, err
	}
	
	// Enable foreign key constraints
	store.DB.Exec("PRAGMA foreign_keys = ON")
	
	// Log successful initialization
	fmt.Println("SQLite database initialized with optimized settings")

	return store, nil
}
