package moderation

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/moderation/image"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

var (
	imageWorker       *image.Worker
	moderationService *image.ModerationService
)

const (
	// Default configuration values
	DefaultCheckInterval = 30 * time.Second
	DefaultConcurrency   = 5
	DefaultTimeout       = 300 * time.Second // Increased to 5 minutes for ML processing
	DefaultTempDir       = "./data/moderation/temp"
	DefaultThreshold     = 0.4
	DefaultMode          = "full"
)

// InitModeration initializes the image moderation system
func InitModeration(store stores.Store, apiEndpoint string, options ...Option) error {
	// Apply default configuration
	config := &Configuration{
		APIEndpoint:   apiEndpoint,
		Threshold:     DefaultThreshold,
		Mode:          DefaultMode,
		Timeout:       DefaultTimeout,
		CheckInterval: DefaultCheckInterval,
		TempDir:       DefaultTempDir,
		Concurrency:   DefaultConcurrency,
		Enabled:       true,
	}

	// Apply custom options
	for _, option := range options {
		option(config)
	}

	// Initialize moderation service
	moderationService = image.NewModerationService(
		config.APIEndpoint,
		config.Threshold,
		config.Mode,
		config.Timeout,
		config.TempDir,
	)

	// Initialize worker if enabled
	if config.Enabled {
		imageWorker = image.NewWorker(
			store,
			moderationService,
			config.CheckInterval,
			config.TempDir,
			config.Concurrency,
		)

		// Start the worker
		imageWorker.Start()
		log.Printf("Image moderation system initialized with API endpoint: %s", config.APIEndpoint)
	} else {
		log.Printf("Image moderation system initialized but disabled")
	}

	return nil
}

// Configuration holds settings for the moderation system
type Configuration struct {
	APIEndpoint   string
	Threshold     float64
	Mode          string
	Timeout       time.Duration
	CheckInterval time.Duration
	TempDir       string
	Concurrency   int
	Enabled       bool
}

// Option represents a configuration option for the moderation system
type Option func(*Configuration)

// WithThreshold sets the confidence threshold for moderation decisions
func WithThreshold(threshold float64) Option {
	return func(c *Configuration) {
		c.Threshold = threshold
	}
}

// WithMode sets the moderation mode (e.g., "full", "fast")
func WithMode(mode string) Option {
	return func(c *Configuration) {
		c.Mode = mode
	}
}

// WithTimeout sets the API request timeout
func WithTimeout(timeout time.Duration) Option {
	return func(c *Configuration) {
		c.Timeout = timeout
	}
}

// WithCheckInterval sets how often the worker checks for pending moderation
func WithCheckInterval(interval time.Duration) Option {
	return func(c *Configuration) {
		c.CheckInterval = interval
	}
}

// WithTempDir sets the directory for temporary file downloads
func WithTempDir(dir string) Option {
	return func(c *Configuration) {
		c.TempDir = dir
	}
}

// WithConcurrency sets the number of concurrent moderation tasks
func WithConcurrency(concurrency int) Option {
	return func(c *Configuration) {
		c.Concurrency = concurrency
	}
}

// WithEnabled enables or disables the moderation system
func WithEnabled(enabled bool) Option {
	return func(c *Configuration) {
		c.Enabled = enabled
	}
}

// Shutdown stops the moderation worker gracefully and cleans up temporary files
func Shutdown() {
	if imageWorker != nil && imageWorker.Running {
		log.Println("Shutting down image moderation worker...")
		imageWorker.Stop()
	}

	// Clean up any temporary files
	if moderationService != nil && moderationService.DownloadDir != "" {
		log.Println("Cleaning up moderation temporary files...")
		cleanupTempFiles(moderationService.DownloadDir)
	}
}

// cleanupTempFiles removes all files from the given directory
func cleanupTempFiles(dir string) {
	if dir == "" {
		return
	}

	// Ensure we don't accidentally delete important directories
	if dir == "/" || dir == "/tmp" || dir == "/var/tmp" {
		log.Println("Refusing to clean potentially system directory:", dir)
		return
	}

	// Read the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("Error reading temp directory %s: %v", dir, err)
		return
	}

	// Delete each file
	var deletedCount int
	for _, file := range files {
		if file.IsDir() {
			continue // Skip subdirectories
		}

		path := filepath.Join(dir, file.Name())
		if err := os.Remove(path); err != nil {
			log.Printf("Error removing temp file %s: %v", path, err)
		} else {
			deletedCount++
		}
	}

	log.Printf("Cleaned up %d temporary files from %s", deletedCount, dir)
}

// GetService returns the initialized moderation service
func GetService() *image.ModerationService {
	return moderationService
}

// GetWorker returns the initialized worker
func GetWorker() *image.Worker {
	return imageWorker
}
