package contentmoderation

import (
	"context"
	"log"
	"time"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/scionic/upload"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// GlobalService is a singleton instance of the content moderation service
var GlobalService *Service

// Initialize sets up the content moderation system
func Initialize(ctx context.Context, db *gorm.DB) (*Service, error) {
	// Load configuration
	config := LoadConfig()

	// Create store
	var store Store
	var err error

	if db != nil {
		// Use database store
		store, err = NewDBStore(db, config)
		if err != nil {
			return nil, err
		}
	} else {
		// Use in-memory store
		store, err = NewInMemoryStore(config)
		if err != nil {
			return nil, err
		}
	}

	// Create service
	service, err := NewService(ctx, config, store)
	if err != nil {
		return nil, err
	}

	// Start service
	if err := service.Start(); err != nil {
		return nil, err
	}

	// Set global service
	GlobalService = service

	log.Printf("Content moderation system initialized with %d workers", config.NumWorkers)
	return service, nil
}

// Shutdown stops the content moderation system
func Shutdown() {
	if GlobalService != nil {
		GlobalService.Stop()
		GlobalService = nil
	}
}

// EnhanceScionicUploadHandler enhances the Scionic upload handler with content moderation
func EnhanceScionicUploadHandler(
	store stores.Store,
	canUploadDag func(rootLeaf *merkle_dag.DagLeaf, pubKey *string, signature *string) bool,
	handleRecievedDag func(dag *merkle_dag.Dag, pubKey *string),
) scionic.UploadDagHandler {
	// The original handler
	originalHandler := upload.BuildUploadStreamHandler(store, canUploadDag, handleRecievedDag)

	// Create our enhanced handler that wraps the original
	enhancedHandler := func(read scionic.UploadDagReader, write scionic.DagWriter) {
		// Use the original handler first
		originalHandler(read, write)

		// After the original handler completes, check if we have a content moderation service
		if GlobalService == nil {
			log.Println("Content moderation service not initialized, skipping moderation")
			return
		}

		// The original handler has already processed the upload
		// We would need access to the DAG data to moderate it
		// This would require modifying the original handler or store
		// For now, we'll just log that moderation should happen
		log.Println("Content moderation would happen here for the uploaded content")
	}

	return enhancedHandler
}

// FilterEvents filters events based on media content moderation
func FilterEvents(events []*nostr.Event) ([]*nostr.Event, error) {
	if GlobalService == nil || len(events) == 0 {
		return events, nil
	}

	var filteredEvents []*nostr.Event

	for _, event := range events {
		safe, err := GlobalService.ProcessEvent(event)
		if err != nil {
			// If we have an "awaiting moderation" error, we could:
			// 1. Filter out the event (conservative)
			// 2. Include it with a warning tag (permissive)
			// 3. Delay processing until moderation completes (complex)

			// For now, we'll take the conservative approach
			if _, ok := err.(*ErrAwaitingModeration); ok {
				// Skip events with media awaiting moderation
				log.Printf("Filtering event %s: media awaiting moderation", event.ID)
				continue
			}

			// For other errors, log and keep the event
			log.Printf("Error processing event %s: %v", event.ID, err)
			filteredEvents = append(filteredEvents, event)
			continue
		}

		if safe {
			// Event is safe to deliver
			filteredEvents = append(filteredEvents, event)
		} else {
			// Event references inappropriate media, filter it
			log.Printf("Filtering event %s: contains inappropriate media", event.ID)
		}
	}

	return filteredEvents, nil
}

// SchedulePeriodicCleanup schedules periodic cleanup of old records
func SchedulePeriodicCleanup(ctx context.Context, interval time.Duration) {
	if GlobalService == nil {
		return
	}

	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Calculate cutoff time
				cutoff := time.Now().Add(-GlobalService.config.RetentionPeriod)
				// Clean up expired records
				if err := GlobalService.store.CleanupExpiredRecords(cutoff); err != nil {
					log.Printf("Error cleaning up expired records: %v", err)
				}
			}
		}
	}()
}

// AddConfigDefaults adds default configuration values for content moderation
func AddConfigDefaults() {
	// API settings
	viper.SetDefault("content_moderation.api_endpoint", "http://localhost:8000/moderate")
	viper.SetDefault("content_moderation.api_timeout", "10s")

	// Processing settings
	viper.SetDefault("content_moderation.num_workers", 5)
	viper.SetDefault("content_moderation.poll_interval", "5s")
	viper.SetDefault("content_moderation.max_attempts", 3)

	// Storage settings
	viper.SetDefault("content_moderation.temp_storage_path", "./temp_media")
	viper.SetDefault("content_moderation.retention_period", "48h")

	// Cache settings
	viper.SetDefault("content_moderation.cache_size", 10000)
	viper.SetDefault("content_moderation.cache_ttl", "24h")

	// Policy settings
	viper.SetDefault("content_moderation.default_mode", "full")
	viper.SetDefault("content_moderation.check_external_media", true)
	viper.SetDefault("content_moderation.external_media_timeout", "5s")
	viper.SetDefault("content_moderation.max_external_size", 10*1024*1024) // 10MB

	// Debug settings
	viper.SetDefault("content_moderation.debug", false)
}
