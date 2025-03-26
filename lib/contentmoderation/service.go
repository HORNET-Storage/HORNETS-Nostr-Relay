package contentmoderation

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/nbd-wtf/go-nostr"
)

// Service is the main content moderation service
type Service struct {
	// Config contains the service configuration
	config *Config

	// Store handles database operations
	store Store

	// APIClient communicates with the moderation API
	apiClient *APIClient

	// WorkerPool processes the moderation queue
	workerPool *WorkerPool

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewService creates a new content moderation service
func NewService(ctx context.Context, config *Config, store Store) (*Service, error) {
	// Create cancellable context
	serviceCtx, cancel := context.WithCancel(ctx)

	// Create API client
	apiClient := NewAPIClient(config)

	// Create service
	service := &Service{
		config:    config,
		store:     store,
		apiClient: apiClient,
		ctx:       serviceCtx,
		cancel:    cancel,
	}

	// Create worker pool
	service.workerPool = NewWorkerPool(serviceCtx, config, store, apiClient)

	return service, nil
}

// Start starts the content moderation service
func (s *Service) Start() error {
	// Create temporary directory if it doesn't exist
	if err := os.MkdirAll(s.config.TempStoragePath, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Initialize the store
	if err := s.store.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}

	// Start the worker pool
	s.workerPool.Start()

	log.Printf("Content moderation service started with %d workers", s.config.NumWorkers)
	return nil
}

// Stop stops the content moderation service
func (s *Service) Stop() error {
	// Stop the worker pool
	s.workerPool.Stop()

	// Cancel the context
	s.cancel()

	// Close the store
	if err := s.store.Close(); err != nil {
		return fmt.Errorf("failed to close store: %w", err)
	}

	log.Printf("Content moderation service stopped")
	return nil
}

// EnqueueMedia adds a new media item to the moderation queue
func (s *Service) EnqueueMedia(record *ModerationRecord, data []byte) error {
	return s.store.EnqueueMedia(record, data)
}

// GetMediaStatus gets the moderation status of a media item
func (s *Service) GetMediaStatus(dagRoot string) (ContentStatus, error) {
	record, err := s.store.GetMediaRecord(dagRoot)
	if err != nil {
		return "", err
	}
	return record.Status, nil
}

// GetEventStatus gets the moderation status of an event
func (s *Service) GetEventStatus(eventID string) (EventStatus, error) {
	return s.store.GetEventStatus(eventID)
}

// ProcessEvent checks an event for media references and processes them
func (s *Service) ProcessEvent(event *nostr.Event) (bool, error) {
	safe, awaitingModeration, err := ProcessEventMediaReferences(s, event)
	if err != nil {
		return false, err
	}

	if !safe {
		// Event contains rejected media
		return false, nil
	}

	if awaitingModeration {
		// Event contains media awaiting moderation
		return false, &ErrAwaitingModeration{EventID: event.ID}
	}

	return true, nil
}

// GetEventMediaReferences gets all media references for an event
func (s *Service) GetEventMediaReferences(eventID string) ([]MediaReferenceInfo, error) {
	return s.store.GetEventMediaReferences(eventID)
}

// MarkEventStatus sets the moderation status for an event
func (s *Service) MarkEventStatus(eventID string, status EventStatus) error {
	return s.store.MarkEventStatus(eventID, status)
}

// GetWorkerPoolStats gets the current worker pool statistics
func (s *Service) GetWorkerPoolStats() WorkerPoolStats {
	return s.workerPool.GetStats()
}
