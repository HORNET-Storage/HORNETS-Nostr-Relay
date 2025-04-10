package sqlite_demo

import (
	"fmt"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	statistics_gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm"
)

// InitStore initializes a statistics store using a dedicated demo database file
// This function is nearly identical to the one in the regular sqlite package
// but uses a different database file specifically for demo purposes
func InitStore(args ...interface{}) (*statistics_gorm.GormStatisticsStore, error) {
	store := &statistics_gorm.GormStatisticsStore{}

	var err error

	// Use demo_statistics.db instead of statistics.db for demo environment
	// The database file will be created in the project root directory if it doesn't exist
	dsn := "demo_statistics.db?_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate&_synchronous=normal&_mutex=no&_locking_mode=normal&cache=shared"

	// Configure GORM with the same settings as the production version
	store.DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:               logger.Default.LogMode(logger.Silent),
		PrepareStmt:          true,
		DisableAutomaticPing: true,
		NowFunc: func() time.Time {
			return time.Now()
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to demo SQLite database: %v", err)
	}

	// Configure connection pool settings
	sqlDB, err := store.DB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %v", err)
	}

	// Use the same connection pool parameters as production
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetConnMaxLifetime(60 * time.Minute)
	sqlDB.SetConnMaxIdleTime(20 * time.Minute)

	// Initialize store schema
	err = store.Init()
	if err != nil {
		return nil, err
	}

	// Set the same PRAGMA settings as production for consistency
	store.DB.Exec("PRAGMA foreign_keys = ON")
	store.DB.Exec("PRAGMA journal_size_limit = 67110000")
	store.DB.Exec("PRAGMA mmap_size = 134217728")
	store.DB.Exec("PRAGMA page_size = 8192")
	store.DB.Exec("PRAGMA cache_size = -32000")
	store.DB.Exec("PRAGMA temp_store = MEMORY")

	fmt.Println("Demo SQLite database initialized with optimized settings")

	return store, nil
}
