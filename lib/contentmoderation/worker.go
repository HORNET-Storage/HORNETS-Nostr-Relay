package contentmoderation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"
)

// Worker is responsible for processing items in the moderation queue
type Worker struct {
	// ID is a unique identifier for this worker
	id int

	// Config contains the worker configuration
	config *Config

	// Store is the data store for moderation records
	store Store

	// APIClient is the client for the moderation API
	apiClient *APIClient

	// processing indicates whether the worker is currently processing an item
	processing bool

	// lastProcessed is the time when the worker last processed an item
	lastProcessed time.Time
}

// WorkerPool manages a pool of workers for processing the moderation queue
type WorkerPool struct {
	// Config contains the worker pool configuration
	config *Config

	// Store is the data store for moderation records
	store Store

	// APIClient is the client for the moderation API
	apiClient *APIClient

	// Workers is the list of workers in the pool
	workers []*Worker

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// WaitGroup for shutting down
	wg sync.WaitGroup

	// Mutex for thread safety
	mu sync.Mutex

	// Stats for monitoring
	stats WorkerPoolStats
}

// WorkerPoolStats contains statistics about the worker pool
type WorkerPoolStats struct {
	// TotalProcessed is the total number of items processed
	TotalProcessed int64

	// ProcessingErrors is the number of processing errors
	ProcessingErrors int64

	// LastProcessedAt is the time when the last item was processed
	LastProcessedAt time.Time

	// CurrentlyProcessing is the number of items currently being processed
	CurrentlyProcessing int

	// TotalApproved is the number of items approved
	TotalApproved int64

	// TotalRejected is the number of items rejected
	TotalRejected int64
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(ctx context.Context, config *Config, store Store, apiClient *APIClient) *WorkerPool {
	poolCtx, cancel := context.WithCancel(ctx)

	pool := &WorkerPool{
		config:    config,
		store:     store,
		apiClient: apiClient,
		ctx:       poolCtx,
		cancel:    cancel,
	}

	// Create workers
	pool.workers = make([]*Worker, config.NumWorkers)
	for i := 0; i < config.NumWorkers; i++ {
		pool.workers[i] = &Worker{
			id:        i,
			config:    config,
			store:     store,
			apiClient: apiClient,
		}
	}

	return pool
}

// Start starts the worker pool
func (p *WorkerPool) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Start each worker
	for _, worker := range p.workers {
		p.wg.Add(1)
		go p.runWorker(worker)
	}

	// Start cleanup goroutine
	p.wg.Add(1)
	go p.runCleanup()

	if p.config.Debug {
		log.Printf("Started content moderation worker pool with %d workers", len(p.workers))
	}
}

// Stop stops the worker pool
func (p *WorkerPool) Stop() {
	p.cancel()
	p.wg.Wait()

	if p.config.Debug {
		log.Printf("Stopped content moderation worker pool")
	}
}

// GetStats returns the current worker pool statistics
func (p *WorkerPool) GetStats() WorkerPoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stats
}

// runWorker runs a worker in a goroutine
func (p *WorkerPool) runWorker(worker *Worker) {
	defer p.wg.Done()

	workerID := worker.id
	log.Printf("Starting worker %d", workerID)

	// Worker loop
	for {
		// Check for cancellation
		select {
		case <-p.ctx.Done():
			log.Printf("Worker %d shutting down", workerID)
			return
		default:
			// Continue processing
		}

		// Try to get the next item
		record, data, err := p.store.GetNextAwaitingMedia()
		if err != nil {
			if err != ErrNoMediaWaiting {
				log.Printf("Error getting next media for worker %d: %v", workerID, err)
			}
			// No media waiting, sleep and try again
			time.Sleep(p.config.PollInterval)
			continue
		}

		// Update stats
		p.mu.Lock()
		p.stats.CurrentlyProcessing++
		p.mu.Unlock()

		// Process the item
		worker.processing = true
		worker.lastProcessed = time.Now()
		if p.config.Debug {
			log.Printf("Worker %d processing %s", workerID, record.DagRoot)
		}

		// Mark worker as processing
		p.processMedia(worker, record, data)

		// Mark worker as done
		worker.processing = false

		// Update stats
		p.mu.Lock()
		p.stats.CurrentlyProcessing--
		p.stats.TotalProcessed++
		p.stats.LastProcessedAt = time.Now()
		p.mu.Unlock()
	}
}

// processMedia processes a media item
func (p *WorkerPool) processMedia(worker *Worker, record *ModerationRecord, data []byte) {
	// Calculate file hash for caching and reference
	fileHash := calculateFileHash(data)

	// Log the file hash in debug mode
	if p.config.Debug {
		log.Printf("Worker %d: Processing %s with hash %s", worker.id, record.DagRoot, fileHash)
	}

	// Determine moderation mode based on content type
	mode := DetermineModerationMode(record.ContentType, p.config)

	// Call the moderation API
	result, err := p.apiClient.ModerateContent(data, mode)
	if err != nil {
		// Handle API error
		log.Printf("Worker %d: Error moderating content %s: %v", worker.id, record.DagRoot, err)

		// Update record with error
		record.ProcessingError = err.Error()
		record.Status = StatusAwaiting // Reset to retry later

		// If we've exceeded max attempts, mark as rejected
		if record.AttemptCount >= p.config.MaxAttempts {
			record.Status = StatusRejected
			record.ModerationLevel = 5 // Highest level for errors

			p.mu.Lock()
			p.stats.TotalRejected++
			p.stats.ProcessingErrors++
			p.mu.Unlock()
		}

		// Save the record
		if err := p.store.UpdateMediaRecord(record); err != nil {
			log.Printf("Worker %d: Error updating record after API error: %v", worker.id, err)
		}
		return
	}

	// Process the result
	now := time.Now()
	record.ModeratedAt = &now
	record.ModerationData = result
	record.ModerationLevel = result.ContentLevel
	record.ProcessingError = "" // Clear any previous errors

	// Determine status based on result
	if result.IsExplicit || result.ContentLevel >= 4 {
		// Content is explicitly inappropriate
		record.Status = StatusRejected

		p.mu.Lock()
		p.stats.TotalRejected++
		p.mu.Unlock()

		if p.config.Debug {
			log.Printf("Worker %d: Rejected content %s (level %d)",
				worker.id, record.DagRoot, result.ContentLevel)
		}
	} else {
		// Content is appropriate
		record.Status = StatusApproved

		p.mu.Lock()
		p.stats.TotalApproved++
		p.mu.Unlock()

		if p.config.Debug {
			log.Printf("Worker %d: Approved content %s (level %d)",
				worker.id, record.DagRoot, result.ContentLevel)
		}
	}

	// Save the updated record
	if err := p.store.UpdateMediaRecord(record); err != nil {
		log.Printf("Worker %d: Error updating record with result: %v", worker.id, err)
		return
	}

	// If rejected, delete the content (delayed to allow for potential disputes)
	if record.Status == StatusRejected {
		// Don't delete immediately, it will be handled by cleanup
		if p.config.Debug {
			log.Printf("Worker %d: Marked content %s for deletion",
				worker.id, record.DagRoot)
		}
	}
}

// runCleanup periodically cleans up old records and files
func (p *WorkerPool) runCleanup() {
	defer p.wg.Done()

	// Clean up every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.performCleanup()
		}
	}
}

// performCleanup cleans up old records and files
func (p *WorkerPool) performCleanup() {
	// Calculate cutoff time based on retention period
	cutoff := time.Now().Add(-p.config.RetentionPeriod)

	if p.config.Debug {
		log.Printf("Cleaning up content moderation records older than %v", cutoff)
	}

	// Clean up expired records
	if err := p.store.CleanupExpiredRecords(cutoff); err != nil {
		log.Printf("Error cleaning up expired records: %v", err)
	}
}

// calculateFileHash calculates a SHA-256 hash of a file
func calculateFileHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// SimulateWorkload can be used for testing the worker pool
func (p *WorkerPool) SimulateWorkload(numItems int) error {
	for i := 0; i < numItems; i++ {
		// Create a dummy record
		record := &ModerationRecord{
			DagRoot:     fmt.Sprintf("test-%d", i),
			ContentType: "image/jpeg",
			FileSize:    1024,
			UploadedBy:  "test",
			UploadedAt:  time.Now(),
		}

		// Create dummy data
		data := make([]byte, 1024)
		for j := 0; j < 1024; j++ {
			data[j] = byte(j % 256)
		}

		// Enqueue the media
		if err := p.store.EnqueueMedia(record, data); err != nil {
			return fmt.Errorf("error enqueueing test media: %w", err)
		}
	}

	return nil
}
