package contentfilter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// ANSI color codes for colorized logging
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"

	// Bold variants
	ColorRedBold    = "\033[1;31m"
	ColorGreenBold  = "\033[1;32m"
	ColorYellowBold = "\033[1;33m"
	ColorBlueBold   = "\033[1;34m"
	ColorPurpleBold = "\033[1;35m"
	ColorCyanBold   = "\033[1;36m"
	ColorWhiteBold  = "\033[1;37m"
)

// Service handles direct communication with Ollama for content filtering
type Service struct {
	ollamaURL   string
	ollamaModel string
	client      *http.Client
	cache       *Cache
	enabled     bool
	filterKind  []int // Event kinds that should be filtered
}

// ServiceConfig defines the configuration options for the filter service
type ServiceConfig struct {
	APIURL     string
	Model      string
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
		config.Timeout = 10 * time.Second
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
	if config.Model == "" {
		config.Model = "gemma3:4b" // Default model
	}

	return &Service{
		ollamaURL:   config.APIURL,
		ollamaModel: config.Model,
		client:      &http.Client{Timeout: config.Timeout},
		cache:       NewCache(config.CacheSize, config.CacheTTL),
		enabled:     config.Enabled,
		filterKind:  config.FilterKind,
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

// FilterEvent filters a single event using Ollama directly
func (s *Service) FilterEvent(event *nostr.Event, instructions string) (FilterResult, error) {
	// Skip if filtering is disabled or event kind shouldn't be filtered
	if !s.enabled || !s.ShouldFilterKind(event.Kind) {
		return FilterResult{Pass: true, Reason: "Filtering skipped"}, nil
	}

	// Generate instruction hash for cache key
	instructionsHash := GenerateInstructionsHash(instructions)

	// Check cache first
	if result, found := s.cache.Get(event.ID, instructionsHash); found {
		log.Printf(ColorCyan+"Cache hit for event %s"+ColorReset, event.ID)
		return result, nil
	}

	// Build prompt for Ollama
	prompt := BuildPrompt(event.Content, instructions)

	// Call Ollama directly
	result, err := s.callOllama(event, prompt)
	if err != nil {
		return FilterResult{Pass: true, Reason: "API error"}, fmt.Errorf("error calling Ollama API: %v", err)
	}

	// Cache the result
	s.cache.Set(event.ID, instructionsHash, result)
	if result.Pass {
		log.Printf(ColorGreen+"Cached filter result for event %s, pass=%v, reason=%s"+ColorReset, event.ID, result.Pass, result.Reason)
	} else {
		log.Printf(ColorRed+"Cached filter result for event %s, pass=%v, reason=%s"+ColorReset, event.ID, result.Pass, result.Reason)
	}

	return result, nil
}

// callOllama sends a direct request to Ollama API and processes the response
func (s *Service) callOllama(event *nostr.Event, prompt string) (FilterResult, error) {
	// Prepare request payload
	ollamaReq := OllamaRequest{
		Model:     s.ollamaModel,
		Prompt:    prompt,
		Stream:    false,
		MaxTokens: 100, // Limit token generation
	}

	jsonData, err := json.Marshal(ollamaReq)
	if err != nil {
		return FilterResult{Pass: true, Reason: "Error marshaling request"}, fmt.Errorf("error marshaling Ollama request: %v", err)
	}

	// Send request to Ollama
	resp, err := s.client.Post(s.ollamaURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return FilterResult{Pass: true, Reason: "API error"}, fmt.Errorf("error calling Ollama API: %v", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return FilterResult{Pass: true, Reason: "API error"}, fmt.Errorf("ollama API returned status %d", resp.StatusCode)
	}

	// Parse response
	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return FilterResult{Pass: true, Reason: "Error parsing response"}, fmt.Errorf("error parsing Ollama response: %v", err)
	}

	// Log the raw response with distinctive formatting and color
	log.Printf(ColorYellowBold+"[CONTENT FILTER] OLLAMA RESPONSE: %s"+ColorReset, ollamaResp.Response)

	// Process the response to determine true/false
	responseLower := strings.ToLower(strings.TrimSpace(ollamaResp.Response))

	// Generate a more informative reason based on the content
	// Take first 30 chars of content for context
	contentPreview := event.Content
	if len(contentPreview) > 30 {
		contentPreview = contentPreview[:30] + "..."
	}

	// Look for "true" or "false" in the response
	if strings.Contains(responseLower, "true") {
		// For approved content
		return FilterResult{
			Pass:   true,
			Reason: fmt.Sprintf("Included: Content about \"%s\" matches user preferences", contentPreview),
		}, nil
	} else if strings.Contains(responseLower, "false") {
		// For filtered content
		return FilterResult{
			Pass:   false,
			Reason: fmt.Sprintf("Filtered: Content about \"%s\" doesn't match user preferences", contentPreview),
		}, nil
	}

	// If we couldn't find a clear true/false, default to false (safer)
	log.Printf(ColorPurpleBold+"[CONTENT FILTER] UNCLEAR RESPONSE: %s"+ColorReset, ollamaResp.Response)
	return FilterResult{
		Pass:   false,
		Reason: fmt.Sprintf("Default filter: Unclear model response for \"%s\"", contentPreview),
	}, nil
}

// FilterEvents applies filtering to multiple events based on custom instructions
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
				log.Printf(ColorGreen+"[CONTENT FILTER] PASSED: Event %s matches user preferences"+ColorReset, e.ID)
			} else {
				log.Printf(ColorRedBold+"[CONTENT FILTER] FILTERED: Event %s - %s"+ColorReset, e.ID, result.Reason)
			}
		}(event)
	}

	// Wait for all workers to complete
	wg.Wait()

	return filteredEvents, nil
}

// FilterEventsBatch efficiently processes multiple events in batches
func (s *Service) FilterEventsBatch(events []*nostr.Event, instructions string, batchSize int) ([]*nostr.Event, error) {
	if !s.enabled || len(events) == 0 || instructions == "" {
		return events, nil
	}

	// Split events into batches
	batches := splitIntoBatches(events, batchSize)
	log.Printf(ColorCyanBold+"[CONTENT FILTER] BATCH PROCESSING: %d events in %d batches"+ColorReset, len(events), len(batches))

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

			// Process uncached events individually (no API batch endpoint with direct Ollama integration)
			if len(uncachedEvents) > 0 {
				log.Printf("Batch %d: Processing %d uncached events individually", index, len(uncachedEvents))

				// Create a semaphore to limit concurrent API calls
				batchSemaphore := make(chan struct{}, 5)

				// Process each event in parallel
				for _, event := range uncachedEvents {
					wg.Add(1)
					go func(e *nostr.Event) {
						defer wg.Done()

						// Acquire semaphore
						batchSemaphore <- struct{}{}
						defer func() { <-batchSemaphore }()

						// Filter the event
						result, err := s.FilterEvent(e, instructions)
						if err != nil {
							log.Printf("Error filtering event %s in batch %d: %v", e.ID, index, err)
							// On error, pass the event through
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
