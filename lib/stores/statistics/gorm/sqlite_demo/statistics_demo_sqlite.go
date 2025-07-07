package sqlite_demo

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	statistics_gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm"
)

// InitStore initializes a statistics store using a dedicated demo database file
// This function is nearly identical to the one in the regular sqlite package
// but uses a different database file specifically for demo purposes
func InitStore(args ...interface{}) (*statistics_gorm.GormStatisticsStore, error) {
	store := &statistics_gorm.GormStatisticsStore{}

	var err error

	// Find the project root directory by looking for go.mod file
	projectRoot, err := findProjectRoot()
	if err != nil {
		logging.Infof("Warning: Could not determine project root: %v\n", err)
		logging.Infof("Using current directory as a fallback.")
		projectRoot, _ = os.Getwd()
	}

	// Use absolute path to demo_statistics.db in the project root
	dbPath := filepath.Join(projectRoot, "demo_statistics.db")
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate&_synchronous=normal&_mutex=no&_locking_mode=normal&cache=shared"
	logging.Infof("Using database at absolute path: %s\n", dbPath)

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

	logging.Infof("Demo SQLite database initialized with optimized settings")

	return store, nil
}

// findProjectRoot attempts to locate the project root directory by looking for go.mod
func findProjectRoot() (string, error) {
	// Start with the current directory
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %v", err)
	}

	// Traverse up the directory tree looking for go.mod
	for {
		// Check if go.mod exists in the current directory
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, this is likely the project root
			return currentDir, nil
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// We've reached the filesystem root without finding go.mod
			return "", fmt.Errorf("project root not found: go.mod not found in any parent directory")
		}
		currentDir = parentDir
	}
}
