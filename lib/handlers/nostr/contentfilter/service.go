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
