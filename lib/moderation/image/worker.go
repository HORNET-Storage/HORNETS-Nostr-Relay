package image

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr/kind19843"
	stores "github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Worker handles the background processing of media (images and videos) pending moderation
//
// The Worker is responsible for:
// 1. Processing pending moderation events in a race-condition-free manner
// 2. Handling temporary file downloads and cleanup
// 3. Making moderation decisions based on API results
// 4. Marking events as blocked with a 48-hour retention period
// 5. Performing periodic cleanup of blocked events after their retention period
// 6. Cleaning up stale temporary files that might have been leaked
type Worker struct {
	Store             stores.Store
	ModerationService *ModerationService
	CheckInterval     time.Duration
	TempDir           string
	Concurrency       int
	StopChan          chan struct{}
	Running           bool
}

// NewWorker creates a new worker for processing pending moderation events
func NewWorker(store stores.Store, service *ModerationService, interval time.Duration, tempDir string, concurrency int) *Worker {
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	if concurrency <= 0 {
		concurrency = 3 // Default concurrency
	}

	return &Worker{
		Store:             store,
		ModerationService: service,
		CheckInterval:     interval,
		TempDir:           tempDir,
		Concurrency:       concurrency,
		StopChan:          make(chan struct{}),
	}
}

// Start begins the worker process
func (w *Worker) Start() {
	if w.Running {
		return // Don't start multiple times
	}

	w.Running = true

	// Ticker for checking pending moderation events
	eventTicker := time.NewTicker(w.CheckInterval)

	// Ticker for checking pending dispute moderation events
	disputeTicker := time.NewTicker(w.CheckInterval)

	// Ticker for temporary file cleanup (every 1 hour)
	tempCleanupTicker := time.NewTicker(1 * time.Hour)

	// Ticker for blocked events cleanup (daily)
	blockedEventsCleanupTicker := time.NewTicker(24 * time.Hour)

	// Create a worker pool using semaphore pattern
	semaphore := make(chan struct{}, w.Concurrency)

	go func() {
		log.Printf("Starting image moderation worker with check interval %s", w.CheckInterval)
		defer eventTicker.Stop()
		defer disputeTicker.Stop()
		defer tempCleanupTicker.Stop()
		defer blockedEventsCleanupTicker.Stop()

		for {
			select {
			case <-eventTicker.C:
				// Get and remove pending moderation events atomically
				pendingEvents, err := w.Store.GetAndRemovePendingModeration(10) // Process up to 10 events at a time
				if err != nil {
					log.Printf("Error getting pending moderation events: %v", err)
					continue
				}

				if len(pendingEvents) > 0 {
					log.Printf("Processing %d events pending image moderation", len(pendingEvents))
				}

				// Process each pending event
				for _, event := range pendingEvents {
					// Check if event is already blocked - if so, skip processing
					isBlocked, err := w.Store.IsEventBlocked(event.EventID)
					if err != nil {
						log.Printf("Error checking if event %s is blocked: %v", event.EventID, err)
					}

					if isBlocked {
						log.Printf("Skipping event %s which is already blocked", event.EventID)
						continue
					}

					// Use the semaphore to limit concurrency
					semaphore <- struct{}{}

					go func(eventID string, imageURLs []string) {
						defer func() { <-semaphore }() // Release the semaphore when done

						w.processEvent(eventID, imageURLs)
					}(event.EventID, event.ImageURLs)
				}

			case <-disputeTicker.C:
				// Get and remove pending dispute moderation events atomically
				pendingDisputes, err := w.Store.GetAndRemovePendingDisputeModeration(5) // Process up to 5 disputes at a time
				if err != nil {
					log.Printf("Error getting pending dispute moderation events: %v", err)
					continue
				}

				if len(pendingDisputes) > 0 {
					log.Printf("Processing %d disputes pending moderation", len(pendingDisputes))
				}

				// Process each pending dispute
				for _, dispute := range pendingDisputes {
					// Use the semaphore to limit concurrency
					semaphore <- struct{}{}

					go func(dispute lib.PendingDisputeModeration) {
						defer func() { <-semaphore }() // Release the semaphore when done

						w.processDispute(dispute)
					}(dispute)
				}

			case <-tempCleanupTicker.C:
				// Perform periodic cleanup of the temp directory
				if w.TempDir != "" {
					go w.cleanupStaleFiles()
				}

			case <-blockedEventsCleanupTicker.C:
				// Delete blocked events older than 48 hours
				go w.cleanupBlockedEvents()

			case <-w.StopChan:
				log.Println("Stopping image moderation worker")
				return
			}
		}
	}()
}

// cleanupBlockedEvents deletes blocked events older than 48 hours
func (w *Worker) cleanupBlockedEvents() {
	log.Println("Running blocked events cleanup...")

	// Calculate 48 hours in seconds
	age := int64(48 * 60 * 60)

	// Delete blocked events older than the age
	count, err := w.Store.DeleteBlockedEventsOlderThan(age)
	if err != nil {
		log.Printf("Error cleaning up blocked events: %v", err)
		return
	}

	if count > 0 {
		log.Printf("Deleted %d blocked events older than 48 hours", count)
	}
}

// cleanupStaleFiles removes old temporary files that may have been leaked
func (w *Worker) cleanupStaleFiles() {
	if w.TempDir == "" {
		return
	}

	// Protection against deleting critical directories
	if w.TempDir == "/" || w.TempDir == "/tmp" || w.TempDir == "/var/tmp" {
		log.Println("Refusing to clean potentially system directory:", w.TempDir)
		return
	}

	// Get list of files in the temp directory
	files, err := os.ReadDir(w.TempDir)
	if err != nil {
		log.Printf("Error reading temp directory %s: %v", w.TempDir, err)
		return
	}

	// Current time for age comparison
	now := time.Now()
	staleThreshold := 24 * time.Hour // Files older than 24 hours

	// Count of cleaned files
	var deletedCount int

	// Check each file
	for _, file := range files {
		if file.IsDir() {
			continue // Skip subdirectories
		}

		path := filepath.Join(w.TempDir, file.Name())

		// Get file info to check modification time
		info, err := os.Stat(path)
		if err != nil {
			log.Printf("Error getting file info for %s: %v", path, err)
			continue
		}

		// Delete files older than staleThreshold
		if now.Sub(info.ModTime()) > staleThreshold {
			if err := os.Remove(path); err != nil {
				log.Printf("Error removing stale temp file %s: %v", path, err)
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		log.Printf("Periodic cleanup: removed %d stale temporary files from %s", deletedCount, w.TempDir)
	}
}

// Stop ends the worker process
func (w *Worker) Stop() {
	if !w.Running {
		return
	}

	w.Running = false
	w.StopChan <- struct{}{}
}

// processEvent processes a single event with media (images and videos) for moderation
func (w *Worker) processEvent(eventID string, mediaURLs []string) {
	log.Printf("Processing event %s with %d media URLs", eventID, len(mediaURLs))

	var shouldBlock bool
	var blockReason string
	var blockedMediaURL string
	var contentType string
	var pubKey string
	var lastResponse *ModerationResponse

	// Get the event to extract the pubkey using QueryEvents with the event ID
	events, err := w.Store.QueryEvents(nostr.Filter{
		IDs: []string{eventID},
	})
	if err != nil {
		log.Printf("Error retrieving event %s: %v", eventID, err)
		return
	}

	// Check if we found the event
	if len(events) == 0 {
		log.Printf("Event %s not found", eventID)
		return
	}

	// Get the pubkey from the event
	pubKey = events[0].PubKey

	// Create a title caser for proper Unicode handling
	titleCaser := cases.Title(language.English)

	// Process each media URL
	for _, mediaURL := range mediaURLs {
		// Determine the content type based on the URL
		mediaType := "image"
		if isVideoURL(mediaURL) {
			mediaType = "video"
		}

		response, err := w.ModerationService.ModerateURL(mediaURL)
		if err != nil {
			log.Printf("Error moderating %s %s: %v", mediaType, mediaURL, err)
			continue
		}

		// Log the moderation result using proper Unicode-aware title casing
		log.Printf("%s %s moderation result: level=%d decision=%s confidence=%.2f",
			titleCaser.String(mediaType), mediaURL, response.ContentLevel, response.Decision, response.Confidence)

		if response.ShouldBlock() {
			shouldBlock = true
			blockReason = response.Explanation
			blockedMediaURL = mediaURL
			contentType = mediaType
			lastResponse = response // Store the response that triggered the block
			log.Printf("Event %s will be blocked due to %s %s (reason: %s)",
				eventID, mediaType, mediaURL, response.Explanation)
			break // No need to check other media
		}
	}

	// Take action based on moderation results
	if shouldBlock {
		// Mark the event as blocked with current timestamp and details
		// This will retain it for 48 hours before deletion and create a moderation ticket
		timestamp := time.Now().Unix()
		contentLevel := 0

		// Get content level from lastResponse if available
		if lastResponse != nil {
			contentLevel = lastResponse.ContentLevel
		}

		err := w.Store.MarkEventBlockedWithDetails(eventID, timestamp, blockReason, contentLevel, blockedMediaURL)
		if err != nil {
			log.Printf("Error marking event %s as blocked: %v", eventID, err)
		} else {
			log.Printf("Event %s marked as blocked - will be deleted after 48 hours", eventID)
		}

		// Create a moderation notification
		notification := &lib.ModerationNotification{
			PubKey:      pubKey,
			EventID:     eventID,
			Reason:      blockReason,
			CreatedAt:   time.Now(),
			IsRead:      false,
			ContentType: contentType,
			MediaURL:    blockedMediaURL,
		}

		statsStore := w.Store.GetStatsStore()
		if statsStore != nil {
			err = statsStore.CreateModerationNotification(notification)
			if err != nil {
				log.Printf("Error creating moderation notification: %v", err)
			} else {
				log.Printf("Created moderation notification for event %s", eventID)
			}
		} else {
			log.Printf("Stats store not available, can't create notification for event %s", eventID)
		}
	} else {
		log.Printf("Event %s passed moderation, available for queries", eventID)
	}

	// Remove from pending moderation queue regardless of result
	// Since we're using GetAndRemovePendingModeration, the event might already be removed,
	// but we'll try to remove it anyway to be safe
	err = w.Store.RemoveFromPendingModeration(eventID)
	if err != nil {
		// Only log the error if it's not a "not found" error
		if !strings.Contains(err.Error(), "No data found for this key") {
			log.Printf("Error removing event %s from pending moderation: %v", eventID, err)
		}
	}
}

// isVideoURL checks if a URL is likely to be a video based on extension or patterns
func isVideoURL(url string) bool {
	// Check file extensions
	videoExtensions := []string{".mp4", ".webm", ".mov", ".avi", ".mkv", ".m4v", ".ogv", ".mpg", ".mpeg"}
	for _, ext := range videoExtensions {
		if strings.HasSuffix(strings.ToLower(url), ext) {
			return true
		}
	}

	// Check for video hosting patterns
	videoPatterns := []string{
		"nostr.build/v/",
		"v.nostr.build",
		"video.nostr.build",
		"youtube.com/watch",
		"youtu.be/",
		"vimeo.com/",
	}

	for _, pattern := range videoPatterns {
		if strings.Contains(strings.ToLower(url), pattern) {
			return true
		}
	}

	return false
}

// Note: Image downloading functionality is handled directly by the ModerationService,
// which sends URLs to the moderation API rather than downloading images first.

// processDispute processes a single dispute for re-evaluation
func (w *Worker) processDispute(dispute lib.PendingDisputeModeration) {
	log.Printf("Processing dispute %s for event %s", dispute.DisputeID, dispute.EventID)

	// Check if the event is still blocked
	isBlocked, err := w.Store.IsEventBlocked(dispute.EventID)
	if err != nil {
		log.Printf("Error checking if event %s is blocked: %v", dispute.EventID, err)
		return
	}

	if !isBlocked {
		log.Printf("Skipping dispute for event %s which is no longer blocked", dispute.EventID)
		return
	}

	// Re-evaluate the media with dispute-specific parameters
	response, err := w.ModerationService.ModerateDisputeURL(dispute.MediaURL, dispute.DisputeReason)
	if err != nil {
		log.Printf("Error re-evaluating media %s: %v", dispute.MediaURL, err)
		return
	}

	// Log the re-evaluation result
	log.Printf("Dispute re-evaluation result for %s: level=%d decision=%s confidence=%.2f",
		dispute.MediaURL, response.ContentLevel, response.Decision, response.Confidence)

	// Get relay public key and private key from viper config
	relayPubKey := viper.GetString("RelayPubkey")
	relayPrivKey := viper.GetString("private_key")

	// Determine if the dispute should be approved based on the re-evaluation
	approved := !response.ShouldBlock()

	// Create a resolution event using the kind19843 package
	_, err = kind19843.CreateResolutionEvent(
		w.Store,
		dispute.DisputeID,
		dispute.TicketID,
		dispute.EventID,
		dispute.UserPubKey,
		approved,
		response.Explanation,
		relayPubKey,
		relayPrivKey,
	)

	if err != nil {
		log.Printf("Error creating resolution event: %v", err)
		return
	}

	log.Printf("Created resolution event for dispute %s (approved: %v)", dispute.DisputeID, approved)
}
