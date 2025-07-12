package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	statistics_gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm"
)

func InitStore(args ...interface{}) (*statistics_gorm.GormStatisticsStore, error) {
	store := &statistics_gorm.GormStatisticsStore{}
	var err error

	statisticsPath := config.GetPath("statistics")

	if _, err := os.Stat(statisticsPath); os.IsNotExist(err) {
		err := os.Mkdir(statisticsPath, os.ModePerm)
		if err != nil {
			logging.Fatalf("Failed to create statistics directory: %v", err)
		}
	}

	// Configure SQLite with optimal connection handling for concurrent access:
	// - journal_mode=WAL enables Write-Ahead Logging for better concurrency
	// - busy_timeout=30000 waits up to 30 seconds when database is locked (increased from 10s)
	// - _txlock=immediate begins transactions sooner to reduce deadlocks
	// - _synchronous=normal provides a balance of safety and performance
	// - _mutex=no disables recursive mutexes for better concurrency
	// - _locking_mode=normal allows multiple readers
	// - cache=shared enables shared cache mode for better performance
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate&_synchronous=normal&_mutex=no&_locking_mode=normal&cache=shared", filepath.Join(statisticsPath, "statistics.db"))

	// Configure GORM with more advanced settings
	store.DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// Logger configuration to show SQL queries during development
		Logger: logger.Default.LogMode(logger.Silent), // Change to Info for debugging

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

	// Set optimized connection pool parameters
	sqlDB.SetMaxIdleConns(10)                  // Maximum number of idle connections (increased from 5)
	sqlDB.SetMaxOpenConns(30)                  // Maximum number of open connections (increased from 20)
	sqlDB.SetConnMaxLifetime(60 * time.Minute) // Maximum connection lifetime (increased from 30 min)
	sqlDB.SetConnMaxIdleTime(20 * time.Minute) // Maximum idle connection lifetime (increased from 10 min)

	// Initialize store schema
	err = store.Init()
	if err != nil {
		return nil, err
	}

	// Set additional PRAGMA settings for better concurrency
	store.DB.Exec("PRAGMA foreign_keys = ON")
	store.DB.Exec("PRAGMA journal_size_limit = 67110000") // Limit WAL size to ~64MB
	store.DB.Exec("PRAGMA mmap_size = 134217728")         // Use memory mapping for better performance (128MB)
	store.DB.Exec("PRAGMA page_size = 8192")              // Larger pages for better performance
	store.DB.Exec("PRAGMA cache_size = -32000")           // Use a 32MB page cache (negative means KB)
	store.DB.Exec("PRAGMA temp_store = MEMORY")           // Store temporary tables in memory

	return store, nil
}
