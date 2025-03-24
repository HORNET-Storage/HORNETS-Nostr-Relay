package contentfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// Service handles communication with the Nest Feeder API
type Service struct {
	apiURL     string
	client     *http.Client
	cache      *Cache
	enabled    bool
	filterKind []int // Event kinds that should be filtered
}

// ServiceConfig defines the configuration options for the filter service
type ServiceConfig struct {
	APIURL     string
	Timeout    time.Duration
	CacheSize  int
	CacheTTL   time.Duration
	FilterKind []int
	Enabled    bool
}

// NewService creates a new content filter service
func NewService(config ServiceConfig) *Service {
	// Use default values if not provided
	if config.Timeout == 0 {
		config.Timeout = 500 * time.Millisecond
	}
	if config.CacheSize == 0 {
		config.CacheSize = 10000
	}
	if config.CacheTTL == 0 {
		config.CacheTTL = 1 * time.Hour
	}
	if len(config.FilterKind) == 0 {
		config.FilterKind = []int{1} // Default to only filtering kind 1 (text notes)
	}

	return &Service{
		apiURL:     config.APIURL,
		client:     &http.Client{Timeout: config.Timeout},
		cache:      NewCache(config.CacheSize, config.CacheTTL),
		enabled:    config.Enabled,
		filterKind: config.FilterKind,
	}
}

// ShouldFilterKind checks if a given event kind should be filtered
func (s *Service) ShouldFilterKind(kind int) bool {
	for _, k := range s.filterKind {
		if k == kind {
			return true
		}
	}
	return false
}

// FilterEvent filters a single event using the Nest Feeder API
func (s *Service) FilterEvent(event *nostr.Event, instructions string) (FilterResult, error) {
	// Skip if filtering is disabled or event kind shouldn't be filtered
	if !s.enabled || !s.ShouldFilterKind(event.Kind) {
		return FilterResult{Pass: true, Reason: "Filtering skipped"}, nil
	}

	// Generate instruction hash for cache key
	instructionsHash := GenerateInstructionsHash(instructions)

	// Check cache first
	if result, found := s.cache.Get(event.ID, instructionsHash); found {
		log.Printf("Cache hit for event %s", event.ID)
		return result, nil
	}

	// Prepare request payload
	payload := FilterRequest{
		CustomInstruction: instructions,
		EventData:         event,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return FilterResult{Pass: true, Reason: "Error marshaling request"}, fmt.Errorf("error marshaling request: %v", err)
	}

	// Send request to Nest Feeder
	resp, err := s.client.Post(s.apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return FilterResult{Pass: true, Reason: "API error"}, fmt.Errorf("error calling Nest Feeder API: %v", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result FilterResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return FilterResult{Pass: true, Reason: "Error parsing response"}, fmt.Errorf("error parsing API response: %v", err)
	}

	// Cache the result
	s.cache.Set(event.ID, instructionsHash, result)
	log.Printf("Cached filter result for event %s, pass=%v, reason=%s", event.ID, result.Pass, result.Reason)

	return result, nil
}

// FilterEvents applies filtering to a batch of events based on custom instructions
func (s *Service) FilterEvents(events []*nostr.Event, instructions string) ([]*nostr.Event, error) {
	if !s.enabled || instructions == "" {
		return events, nil
	}

	// Determine if we should use batch processing
	// Only use batching when we have enough events to justify it
	const DefaultBatchSize = 20
	if len(events) >= DefaultBatchSize {
		return s.FilterEventsBatch(events, instructions, DefaultBatchSize)
	}

	// For smaller sets, use the original single-event processing approach
	var filteredEvents []*nostr.Event
	var mutex sync.Mutex
	var wg sync.WaitGroup

	// Use a semaphore to limit concurrent API requests
	semaphore := make(chan struct{}, 10)

	// Process each event
	for _, event := range events {
		// Skip filtering for event kinds we don't filter
		if !s.ShouldFilterKind(event.Kind) {
			mutex.Lock()
			filteredEvents = append(filteredEvents, event)
			mutex.Unlock()
			continue
		}

		wg.Add(1)
		go func(e *nostr.Event) {
			defer wg.Done()

			// Acquire semaphore (limit concurrency)
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := s.FilterEvent(e, instructions)
			if err != nil {
				log.Printf("Error filtering event %s: %v", e.ID, err)
				mutex.Lock()
				filteredEvents = append(filteredEvents, e)
				mutex.Unlock()
				return
			}

			// Only include events that pass the filter
			if result.Pass {
				mutex.Lock()
				filteredEvents = append(filteredEvents, e)
				mutex.Unlock()
			} else {
				log.Printf("Filtered out event %s: %s", e.ID, result.Reason)
			}
		}(event)
	}

	// Wait for all workers to complete
	wg.Wait()

	return filteredEvents, nil
}

// FilterEventsBatch sends a batch of events to be filtered with the same instructions
func (s *Service) FilterEventsBatch(events []*nostr.Event, instructions string, batchSize int) ([]*nostr.Event, error) {
	if !s.enabled || len(events) == 0 || instructions == "" {
		return events, nil
	}

	// Split events into batches
	batches := splitIntoBatches(events, batchSize)
	log.Printf("Processing %d events in %d batches", len(events), len(batches))

	var filteredEvents []*nostr.Event
	var mutex sync.Mutex
	var wg sync.WaitGroup

	// First, extract and handle non-filterable events immediately
	for _, event := range events {
		if !s.ShouldFilterKind(event.Kind) {
			mutex.Lock()
			filteredEvents = append(filteredEvents, event)
			mutex.Unlock()
		}
	}

	// Process each batch
	for batchIndex, batch := range batches {
		wg.Add(1)
		go func(index int, batchEvents []*nostr.Event) {
			defer wg.Done()

			// Filter out events that shouldn't be filtered
			var filteredBatch []*nostr.Event
			for _, event := range batchEvents {
				if s.ShouldFilterKind(event.Kind) {
					filteredBatch = append(filteredBatch, event)
				}
			}

			if len(filteredBatch) == 0 {
				return // Nothing to process in this batch
			}

			// Check cache first for each event in batch
			var uncachedEvents []*nostr.Event
			var cachedResults = make(map[string]FilterResult)
			var eventMap = make(map[string]*nostr.Event)

			instructionsHash := GenerateInstructionsHash(instructions)
			for _, event := range filteredBatch {
				eventMap[event.ID] = event
				if result, found := s.cache.Get(event.ID, instructionsHash); found {
					log.Printf("Cache hit for event %s in batch %d", event.ID, index)
					cachedResults[event.ID] = result
				} else {
					uncachedEvents = append(uncachedEvents, event)
				}
			}

			// If we have uncached events, make a batch API call
			if len(uncachedEvents) > 0 {
				log.Printf("Batch %d: Making API call for %d uncached events", index, len(uncachedEvents))
				results, err := s.makeBatchAPICall(uncachedEvents, instructions)
				if err != nil {
					// On error, pass all events through
					log.Printf("Error in batch API call for batch %d: %v", index, err)
					mutex.Lock()
					filteredEvents = append(filteredEvents, filteredBatch...)
					mutex.Unlock()
					return
				}

				// Process results and update cache
				for i, event := range uncachedEvents {
					if i < len(results) {
						// Store in cache
						s.cache.Set(event.ID, instructionsHash, results[i])

						// Add to filtered events if it passes
						if results[i].Pass {
							mutex.Lock()
							filteredEvents = append(filteredEvents, event)
							mutex.Unlock()
						} else {
							log.Printf("Filtered out event %s: %s", event.ID, results[i].Reason)
						}
					} else {
						// If we got fewer results than events, pass the remaining events through
						log.Printf("Missing result for event %s in batch response, passing through", event.ID)
						mutex.Lock()
						filteredEvents = append(filteredEvents, event)
						mutex.Unlock()
					}
				}
			}

			// Process cached results
			for eventID, result := range cachedResults {
				if result.Pass {
					if event, exists := eventMap[eventID]; exists {
						mutex.Lock()
						filteredEvents = append(filteredEvents, event)
						mutex.Unlock()
					}
				} else {
					log.Printf("Filtered out event %s (from cache): %s", eventID, result.Reason)
				}
			}
		}(batchIndex, batch)
	}

	wg.Wait()
	return filteredEvents, nil
}

// makeBatchAPICall sends a batch of events to the API
func (s *Service) makeBatchAPICall(events []*nostr.Event, instructions string) ([]FilterResult, error) {
	// Prepare batch request payload
	payload := BatchFilterRequest{
		CustomInstruction: instructions,
		Events:            make([]interface{}, len(events)),
	}

	// Convert events to interface{}
	for i, event := range events {
		payload.Events[i] = event
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error marshaling batch request: %v", err)
	}

	// Send request to Nest Feeder API
	resp, err := s.client.Post(s.apiURL+"/batch", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		// If batch endpoint fails, try falling back to processing events individually
		log.Printf("Batch API call failed, falling back to individual processing: %v", err)
		return s.fallbackToIndividualProcessing(events, instructions)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		log.Printf("Batch API returned non-OK status: %d", resp.StatusCode)
		return s.fallbackToIndividualProcessing(events, instructions)
	}

	// Parse batch response
	var batchResponse BatchFilterResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResponse); err != nil {
		return nil, fmt.Errorf("error parsing batch API response: %v", err)
	}

	// Ensure we have results for all events
	if len(batchResponse.Results) != len(events) {
		log.Printf("Warning: Received %d results for %d events", len(batchResponse.Results), len(events))
		// Pad the results if needed
		for len(batchResponse.Results) < len(events) {
			batchResponse.Results = append(batchResponse.Results, FilterResult{
				Pass:   true,
				Reason: "Missing result in batch response",
			})
		}
	}

	return batchResponse.Results, nil
}

// fallbackToIndividualProcessing processes events individually when the batch API fails
func (s *Service) fallbackToIndividualProcessing(events []*nostr.Event, instructions string) ([]FilterResult, error) {
	var results []FilterResult
	var wg sync.WaitGroup
	var mutex sync.Mutex
	semaphore := make(chan struct{}, 5) // Limit concurrency for fallback

	// Process events in parallel
	for _, event := range events {
		wg.Add(1)
		go func(e *nostr.Event) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Use single event processing
			result, err := s.FilterEvent(e, instructions)
			if err != nil {
				log.Printf("Error in fallback processing for event %s: %v", e.ID, err)
				// On error, let the event pass
				mutex.Lock()
				results = append(results, FilterResult{
					Pass:   true,
					Reason: "Error in fallback processing",
				})
				mutex.Unlock()
				return
			}

			mutex.Lock()
			results = append(results, result)
			mutex.Unlock()
		}(event)
	}

	wg.Wait()
	return results, nil
}

// Helper function to split events into batches
func splitIntoBatches(events []*nostr.Event, batchSize int) [][]*nostr.Event {
	if batchSize <= 0 {
		batchSize = 20 // Default batch size
	}

	// Calculate number of batches
	numBatches := (len(events) + batchSize - 1) / batchSize // Ceiling division

	// Create batch slices
	batches := make([][]*nostr.Event, numBatches)
	for i := 0; i < numBatches; i++ {
		start := i * batchSize
		end := start + batchSize
		if end > len(events) {
			end = len(events)
		}
		batches[i] = events[start:end]
	}

	return batches
}

// RunPeriodicCacheCleanup starts a goroutine that periodically cleans up expired cache entries
func (s *Service) RunPeriodicCacheCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			s.cache.Cleanup()
			log.Printf("Cache cleanup completed, current size: %d", s.cache.Size())
		}
	}()
}
