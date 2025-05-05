package xnostr

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/signing"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// NitterInstance represents a Nitter instance with health tracking
type NitterInstance struct {
	URL           string
	Priority      int
	SuccessCount  int
	FailureCount  int
	LastSuccess   time.Time
	LastFailure   time.Time
	ResponseTimes []time.Duration // Last N response times
	Disabled      bool
}

// Service handles X-Nostr verification
type Service struct {
	tempDir           string
	browserPath       string
	updateInterval    time.Duration
	browser           *rod.Browser
	mutex             sync.Mutex // For thread-safe browser access
	browserPool       []*rod.Browser
	browserPoolSize   int
	browserPoolMutex  sync.Mutex // For thread-safe browser pool access
	nitterInstances   []*NitterInstance
	nitterMutex       sync.RWMutex // For thread-safe Nitter instance access
	requestsPerMinute int
	lastRequestTime   time.Time
	requestCounter    int
	initialized       bool      // Track if browser is initialized
	initOnce          sync.Once // Ensure initialization happens only once
}

// NewService creates a new X-Nostr verification service
func NewService(tempDir string, browserPath string, updateInterval time.Duration) *Service {
	// Create temp directory if it doesn't exist
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		os.MkdirAll(tempDir, 0755)
	}

	// Default Nitter instances with priorities
	defaultNitterInstances := []*NitterInstance{
		{URL: "https://nitter.net/", Priority: 1},
		{URL: "https://nitter.lacontrevoie.fr/", Priority: 2},
		{URL: "https://nitter.1d4.us/", Priority: 3},
		{URL: "https://nitter.kavin.rocks/", Priority: 4},
		{URL: "https://nitter.unixfox.eu/", Priority: 5},
		{URL: "https://nitter.fdn.fr/", Priority: 6},
		{URL: "https://nitter.pussthecat.org/", Priority: 7},
		{URL: "https://nitter.nixnet.services/", Priority: 8},
	}

	return &Service{
		tempDir:           tempDir,
		browserPath:       browserPath,
		updateInterval:    updateInterval,
		mutex:             sync.Mutex{},
		browserPool:       make([]*rod.Browser, 0, 3), // Default pool size of 3
		browserPoolSize:   3,                          // Default pool size
		browserPoolMutex:  sync.Mutex{},
		nitterInstances:   defaultNitterInstances,
		nitterMutex:       sync.RWMutex{},
		requestsPerMinute: 10, // Default rate limit
		lastRequestTime:   time.Now(),
		requestCounter:    0,
	}
}

// SetBrowserPoolSize sets the maximum size of the browser pool
func (s *Service) SetBrowserPoolSize(size int) {
	if size <= 0 {
		size = 3 // Default to 3 if invalid size is provided
	}
	s.browserPoolMutex.Lock()
	defer s.browserPoolMutex.Unlock()
	s.browserPoolSize = size
}

// SetNitterInstances sets the list of Nitter instances to use
func (s *Service) SetNitterInstances(instances []*NitterInstance) {
	s.nitterMutex.Lock()
	defer s.nitterMutex.Unlock()
	s.nitterInstances = instances
}

// SetRequestsPerMinute sets the rate limit for requests
func (s *Service) SetRequestsPerMinute(rpm int) {
	s.nitterMutex.Lock()
	defer s.nitterMutex.Unlock()
	s.requestsPerMinute = rpm
}

// GetNextNitterInstance returns the next available Nitter instance based on health and priority
func (s *Service) GetNextNitterInstance() *NitterInstance {
	s.nitterMutex.RLock()
	defer s.nitterMutex.RUnlock()

	// Apply rate limiting
	now := time.Now()
	if now.Sub(s.lastRequestTime) > time.Minute {
		// Reset counter if more than a minute has passed
		s.requestCounter = 0
		s.lastRequestTime = now
	} else if s.requestCounter >= s.requestsPerMinute {
		// If we've exceeded the rate limit, wait until the minute is up
		time.Sleep(time.Minute - now.Sub(s.lastRequestTime))
		s.requestCounter = 0
		s.lastRequestTime = time.Now()
	}
	s.requestCounter++

	// First, try to find a healthy instance (more successes than failures)
	var bestInstance *NitterInstance
	var bestScore float64 = -1

	for _, instance := range s.nitterInstances {
		if instance.Disabled {
			continue
		}

		// Calculate a health score based on success rate and priority
		// Lower priority number means higher priority
		successRate := 0.0
		if instance.SuccessCount+instance.FailureCount > 0 {
			successRate = float64(instance.SuccessCount) / float64(instance.SuccessCount+instance.FailureCount)
		}

		// Prioritize instances with recent successes
		recencyBonus := 0.0
		if !instance.LastSuccess.IsZero() {
			// Give bonus for recent successes (within last hour)
			hoursSinceSuccess := time.Since(instance.LastSuccess).Hours()
			if hoursSinceSuccess < 1 {
				recencyBonus = 0.2 * (1 - hoursSinceSuccess)
			}
		}

		// Calculate final score (higher is better)
		// Priority is inverted (lower priority number = higher actual priority)
		priorityFactor := 10.0 / float64(instance.Priority+1)
		score := (successRate + recencyBonus) * priorityFactor

		if score > bestScore {
			bestScore = score
			bestInstance = instance
		}
	}

	// If we found a good instance, return it
	if bestInstance != nil {
		return bestInstance
	}

	// If all instances are disabled or have poor health, return the highest priority one that's not disabled
	for _, instance := range s.nitterInstances {
		if !instance.Disabled {
			return instance
		}
	}

	// If all instances are disabled, enable the highest priority one and return it
	if len(s.nitterInstances) > 0 {
		s.nitterInstances[0].Disabled = false
		return s.nitterInstances[0]
	}

	// This should never happen if we have default instances
	return nil
}

// UpdateInstanceHealth updates the health metrics for a Nitter instance
func (s *Service) UpdateInstanceHealth(instance *NitterInstance, success bool, responseTime time.Duration) {
	s.nitterMutex.Lock()
	defer s.nitterMutex.Unlock()

	// Find the instance in our list
	var targetInstance *NitterInstance
	for _, inst := range s.nitterInstances {
		if inst.URL == instance.URL {
			targetInstance = inst
			break
		}
	}

	if targetInstance == nil {
		// Instance not found, nothing to update
		return
	}

	// Update success/failure counts and timestamps
	if success {
		targetInstance.SuccessCount++
		targetInstance.LastSuccess = time.Now()

		// Keep track of response times (last 10)
		if len(targetInstance.ResponseTimes) >= 10 {
			targetInstance.ResponseTimes = targetInstance.ResponseTimes[1:]
		}
		targetInstance.ResponseTimes = append(targetInstance.ResponseTimes, responseTime)

		// If this was a success after being disabled, re-enable it
		if targetInstance.Disabled && targetInstance.SuccessCount > targetInstance.FailureCount {
			targetInstance.Disabled = false
		}
	} else {
		targetInstance.FailureCount++
		targetInstance.LastFailure = time.Now()

		// Disable instance if it has failed too many times in a row
		// We consider it "in a row" if there have been no successes in the last hour
		if targetInstance.LastSuccess.IsZero() || time.Since(targetInstance.LastSuccess) > time.Hour {
			if targetInstance.FailureCount >= 3 {
				targetInstance.Disabled = true
			}
		}
	}
}

// Start prepares the X-Nostr verification service without initializing the browser
func (s *Service) Start() {
	log.Printf("Starting X-Nostr verification service (lazy initialization enabled)...")

	// Create temp directory if it doesn't exist
	if _, err := os.Stat(s.tempDir); os.IsNotExist(err) {
		log.Printf("Creating temp directory: %s", s.tempDir)
		if err := os.MkdirAll(s.tempDir, 0755); err != nil {
			log.Printf("Failed to create temp directory %s: %v", s.tempDir, err)
			// Fall back to system temp dir if we can't create our own
			s.tempDir = os.TempDir()
			log.Printf("Falling back to system temp directory: %s", s.tempDir)
		}
	}

	// Browser will be initialized on first use
	log.Printf("Browser will be initialized on first verification request")
}

// ensureInitialized makes sure the browser is initialized before use
func (s *Service) ensureInitialized() {
	s.initOnce.Do(func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()

		if !s.initialized {
			log.Printf("Performing lazy initialization of X-Nostr browser...")

			// Start browser initialization with timeout
			initCtx, initCancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer initCancel()

			// Channel to communicate when initialization is done
			done := make(chan bool, 1)

			// Launch browser initialization in separate goroutine
			go func() {
				// Initialize the browser
				s.initBrowser()

				// Verify initialization was successful
				if s.browser == nil {
					log.Printf("Browser initialization failed, retrying with auto-detection...")
					// Try again with a different browser path
					s.browserPath = "" // Force auto-detection
					s.initBrowser()
				}

				// Signal that initialization is done
				done <- true
			}()

			// Wait for either completion or timeout
			select {
			case <-done:
				if s.browser == nil {
					log.Printf("Browser initialization failed completely. Service may not work properly.")
				} else {
					log.Printf("Browser initialized successfully.")
					s.initialized = true
				}
			case <-initCtx.Done():
				log.Printf("Browser initialization timed out after 120 seconds. Service may not work properly.")
			}
		}
	})
}

// GetBrowser returns a browser instance from the pool or initializes a new one if needed
func (s *Service) GetBrowser() *rod.Browser {
	// Ensure browser is initialized
	s.ensureInitialized()

	// First try to get a browser from the pool
	s.browserPoolMutex.Lock()
	defer s.browserPoolMutex.Unlock()

	// If there's a browser in the pool, use it
	if len(s.browserPool) > 0 {
		browser := s.browserPool[len(s.browserPool)-1]
		s.browserPool = s.browserPool[:len(s.browserPool)-1]

		// Check if browser is still healthy
		if s.isBrowserHealthy(browser) {
			log.Printf("Using browser from pool, %d browsers remaining in pool", len(s.browserPool))
			return browser
		}

		// If not healthy, close it and continue to create a new one
		log.Printf("Browser from pool is not healthy, closing it")
		if browser != nil {
			_ = browser.Close()
		}
	}

	// If no browser in pool or the one we got is not healthy, create a new one
	s.mutex.Lock()
	defer s.mutex.Unlock()

	log.Printf("Creating new browser instance")
	s.initBrowser()
	return s.browser
}

// ReleaseBrowser returns a browser to the pool or closes it if the pool is full
func (s *Service) ReleaseBrowser(browser *rod.Browser) {
	if browser == nil {
		return
	}

	s.browserPoolMutex.Lock()
	defer s.browserPoolMutex.Unlock()

	// Check if browser is still healthy
	if !s.isBrowserHealthy(browser) {
		log.Printf("Browser is not healthy, closing instead of returning to pool")
		_ = browser.Close()
		return
	}

	// If pool is full, close the browser
	if len(s.browserPool) >= s.browserPoolSize {
		log.Printf("Browser pool is full, closing browser instead of returning to pool")
		_ = browser.Close()
		return
	}

	// Add browser back to pool
	log.Printf("Returning browser to pool, pool size now %d", len(s.browserPool)+1)
	s.browserPool = append(s.browserPool, browser)
}

// isBrowserHealthy checks if the given browser is still responsive
func (s *Service) isBrowserHealthy(browser *rod.Browser) bool {
	if browser == nil {
		return false
	}

	// Create a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create a channel to communicate the result
	resultChan := make(chan bool, 1)
	errChan := make(chan error, 1)

	// Try a simple operation to check browser health
	go func() {
		// Try to create a test page
		_, err := browser.Page(proto.TargetCreateTarget{})
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- true
	}()

	// Wait for either a result or timeout
	select {
	case <-resultChan:
		return true
	case err := <-errChan:
		log.Printf("Browser health check failed: %v", err)
		return false
	case <-ctx.Done():
		log.Printf("Browser health check timed out")
		return false
	}
}

// createSafePageWithBrowser creates a page in the provided browser with proper timeout handling
func (s *Service) createSafePageWithBrowser(browser *rod.Browser, timeout time.Duration) (*rod.Page, context.CancelFunc, error) {
	if browser == nil {
		return nil, nil, fmt.Errorf("browser is nil")
	}

	// Use a longer timeout for page creation (3x the requested timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout*3)

	// Create a new page with retry mechanism
	var page *rod.Page
	var err error

	// Try multiple times with exponential backoff
	for retries := 0; retries < 3; retries++ {
		log.Printf("Attempting to create page (attempt %d)...", retries+1)

		// Use a separate timeout just for page creation
		createCtx, createCancel := context.WithTimeout(ctx, 30*time.Second)

		// Create page in a goroutine with timeout protection
		pageChannel := make(chan *rod.Page, 1)
		errChannel := make(chan error, 1)

		go func() {
			p, err := browser.Page(proto.TargetCreateTarget{})
			if err != nil {
				errChannel <- err
				return
			}
			pageChannel <- p
		}()

		// Wait for either the page to be created or timeout
		var success bool
		select {
		case page = <-pageChannel:
			createCancel()
			log.Printf("Page created successfully on attempt %d", retries+1)
			err = nil
			success = true
		case err = <-errChannel:
			createCancel()
			log.Printf("Page creation failed on attempt %d: %v. Retrying...", retries+1, err)
			time.Sleep(time.Duration(1<<uint(retries)) * time.Second) // Exponential backoff
			continue
		case <-createCtx.Done():
			createCancel()
			err = fmt.Errorf("context deadline exceeded")
			log.Printf("Page creation timed out on attempt %d. Retrying...", retries+1)
			time.Sleep(time.Duration(1<<uint(retries)) * time.Second) // Exponential backoff
			continue
		}

		// If we got a page successfully, break out of the retry loop
		if success {
			break
		}
	}

	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create page after retries: %v", err)
	}

	if page == nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to create page: page is nil after attempts")
	}

	// Return the page with the context
	return page.Context(ctx), cancel, nil
}

// VerifyProfile verifies a Nostr profile against X using Nitter instances
func (s *Service) VerifyProfile(pubkey, xHandle string) (*VerificationResult, error) {
	// Ensure browser is initialized
	s.ensureInitialized()

	// Generate temporary filenames with unique timestamp
	timestamp := time.Now().Unix()
	screenshotPath := filepath.Join(s.tempDir, fmt.Sprintf("%s_%d.png", xHandle, timestamp))
	jsonPath := filepath.Join(s.tempDir, fmt.Sprintf("%s_%d.json", xHandle, timestamp))

	// Ensure temporary directory exists
	dirErr := os.MkdirAll(filepath.Dir(screenshotPath), 0755)
	if dirErr != nil {
		log.Printf("Error creating screenshot directory: %v", dirErr)
		// continue anyway but log the error
	}

	// Try Nitter instances
	nitterSuccess, dataExtracted, err := s.verifyWithNitter(xHandle, screenshotPath, jsonPath)
	if err != nil {
		log.Printf("Error accessing Nitter profiles: %v", err)
	}

	if !nitterSuccess {
		log.Printf("All Nitter instances failed for %s", xHandle)
		return &VerificationResult{
			IsVerified:         false,
			FollowerCount:      "",
			VerifiedAt:         time.Now(),
			VerificationSource: "none",
			Error:              "Failed to verify: could not access any Nitter profile",
		}, nil
	}

	if !dataExtracted {
		// Check if JSON file exists
		_, statErr := os.Stat(jsonPath)
		if os.IsNotExist(statErr) {
			log.Printf("JSON profile data file not found: %s", jsonPath)
			return &VerificationResult{
				IsVerified:         false,
				FollowerCount:      "",
				VerifiedAt:         time.Now(),
				VerificationSource: "none",
				Error:              "Failed to verify: no profile data available",
			}, nil
		}
	}

	// Read the JSON file to get the profile data
	profileData, err := s.readProfileData(jsonPath)
	if err != nil {
		log.Printf("Failed to read profile data: %v", err)
		return &VerificationResult{
			IsVerified:         false,
			FollowerCount:      "",
			VerifiedAt:         time.Now(),
			VerificationSource: "none",
			Error:              fmt.Sprintf("Failed to read profile data: %v", err),
		}, nil
	}

	// Check if the profile contains the npub
	verified := false
	verificationSource := ""

	if profileData.Npub != "" {
		// Convert the npub to hex format for comparison
		npubBytes, err := signing.DecodeKey(profileData.Npub)
		if err == nil {
			// Convert bytes to hex string
			npubHex := hex.EncodeToString(npubBytes)
			// Compare the hex pubkeys directly
			verified = npubHex == pubkey
			verificationSource = "bio"
			log.Printf("Bio verification comparison: npubHex=%s, pubkey=%s, match=%t", npubHex, pubkey, verified)
		} else {
			log.Printf("Error decoding npub: %v", err)
		}
	} else {
		// If npub not found in bio, search for tweets with #MyNostrKey: hashtag
		log.Printf("No npub found in bio for %s, searching for tweets with #MyNostrKey: hashtag", xHandle)
		npub, err := s.searchNitterForNostrKeyTweet(xHandle)
		if err != nil {
			log.Printf("Error searching for #MyNostrKey tweets: %v", err)
		} else if npub != "" {
			log.Printf("Found npub in tweet with #MyNostrKey: hashtag: %s", npub)
			profileData.Npub = npub
			verificationSource = "tweet"

			// Convert the npub to hex format for comparison
			npubBytes, err := signing.DecodeKey(npub)
			if err == nil {
				// Convert bytes to hex string
				npubHex := hex.EncodeToString(npubBytes)
				// Compare the hex pubkeys directly
				verified = npubHex == pubkey
				log.Printf("Tweet verification comparison: npubHex=%s, pubkey=%s, match=%t", npubHex, pubkey, verified)
			} else {
				log.Printf("Error decoding npub from tweet: %v", err)
			}
		}
	}

	result := &VerificationResult{
		IsVerified:         verified,
		FollowerCount:      profileData.FollowerCount,
		VerifiedAt:         time.Now(),
		VerificationSource: verificationSource,
	}

	// Clean up temporary files
	cleanupErr1 := os.Remove(screenshotPath)
	cleanupErr2 := os.Remove(jsonPath)
	if cleanupErr1 != nil || cleanupErr2 != nil {
		log.Printf("Warning: Failed to clean up temporary files: %v, %v", cleanupErr1, cleanupErr2)
	}

	return result, nil
}

// verifyWithNitter attempts to verify a profile using Nitter instances
func (s *Service) verifyWithNitter(xHandle, screenshotPath, jsonOutputFile string) (bool, bool, error) {
	if s.browser == nil {
		return false, false, fmt.Errorf("browser not initialized")
	}

	log.Println("Verifying profile using Nitter instances...")

	// Track overall success
	var nitterSuccess bool
	var dataExtracted bool
	var lastError error

	// Get a single browser from the pool to use for all Nitter instances
	// This avoids initializing a new browser for each instance
	browser := s.GetBrowser()
	if browser == nil {
		log.Printf("Failed to get browser from pool")
		return false, false, fmt.Errorf("failed to get browser from pool")
	}
	// Make sure we return the browser to the pool when we're done
	defer s.ReleaseBrowser(browser)

	// Try up to 3 different Nitter instances
	for attempts := 0; attempts < 3; attempts++ {
		// Get the next best Nitter instance
		instance := s.GetNextNitterInstance()
		if instance == nil {
			log.Println("No available Nitter instances")
			return false, false, fmt.Errorf("no available Nitter instances")
		}

		nitterURL := instance.URL + xHandle
		log.Printf("Trying Nitter instance: %s\n", nitterURL)

		startTime := time.Now()

		// Create a new page with proper error handling and timeout
		// Using the same browser instance for all pages
		nitterPage, cancel, err := s.createSafePageWithBrowser(browser, 30*time.Second)
		if err != nil {
			log.Printf("Error creating page for Nitter: %v", err)
			s.UpdateInstanceHealth(instance, false, time.Since(startTime))
			lastError = err
			continue
		}

		var pageClosedInFunc bool
		var instanceSuccess bool

		// Use a try-catch approach with recover to handle potential panics
		func() {
			defer cancel()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic while loading Nitter instance %s: %v\n", nitterURL, r)
				}
				if !pageClosedInFunc && nitterPage != nil {
					nitterPage.Close()
					pageClosedInFunc = true
				}

				// Update instance health
				s.UpdateInstanceHealth(instance, instanceSuccess, time.Since(startTime))
			}()

			// Navigate to the URL with timeout
			err := nitterPage.Navigate(nitterURL)
			if err != nil {
				log.Printf("Error navigating to Nitter instance %s: %v\n", nitterURL, err)
				return
			}

			// Wait for page to load with timeout
			err = nitterPage.WaitLoad()
			if err != nil {
				log.Printf("Timeout waiting for Nitter instance %s to load: %v\n", nitterURL, err)
				return
			}

			// Check if the page loaded successfully and contains profile data
			profileFound := false

			// Try different selectors that might indicate a profile page
			nitterSelectors := []string{
				".profile-card",
				".profile-statlist",
				".profile-stat",
				".timeline-item",
			}

			for _, selector := range nitterSelectors {
				elements, err := nitterPage.Elements(selector)
				if err == nil && len(elements) > 0 {
					profileFound = true
					break
				}
			}

			if !profileFound {
				log.Printf("Profile elements not found on Nitter instance %s\n", nitterURL)
				return
			}

			// Wait a moment for content to fully load
			time.Sleep(2 * time.Second)

			// Extract HTML for analysis
			profileHTML := s.captureProfileHTML(nitterPage)
			statsHTML := s.captureStatsHTML(nitterPage)

			// Try to extract profile data using LLM from the HTML
			htmlProfileData, err := s.extractProfileDataFromHTML(profileHTML, statsHTML)
			if err == nil && (htmlProfileData.FollowerCount != "" || htmlProfileData.Npub != "") {
				log.Println("Successfully extracted profile data from HTML")
				log.Printf("Extracted data: npub=%v, follower_count=%v\n", htmlProfileData.Npub, htmlProfileData.FollowerCount)

				// Save the extracted data to JSON
				saveErr := s.saveProfileData(htmlProfileData, jsonOutputFile)
				if saveErr != nil {
					log.Printf("Error saving profile data: %v", saveErr)
				}

				// Save the screenshot for reference
				img, screenshotErr := nitterPage.Screenshot(false, &proto.PageCaptureScreenshot{})
				if screenshotErr != nil {
					log.Printf("Error taking screenshot: %v", screenshotErr)
				} else {
					if err := saveFile(img, screenshotPath); err != nil {
						log.Printf("Error saving screenshot: %v", err)
					}
				}

				instanceSuccess = true
				nitterSuccess = true
				dataExtracted = true
				return
			}

			// If HTML extraction failed, try direct DOM extraction
			profileData, err := s.extractProfileDataFromDOM(nitterPage)
			if err == nil && (profileData.FollowerCount != "" || profileData.Npub != "") {
				log.Println("Successfully extracted profile data directly from Nitter DOM")
				log.Printf("Extracted data: npub=%v, follower_count=%v\n", profileData.Npub, profileData.FollowerCount)

				// Save the screenshot for reference
				img, screenshotErr := nitterPage.Screenshot(false, &proto.PageCaptureScreenshot{})
				if screenshotErr != nil {
					log.Printf("Error taking screenshot: %v", screenshotErr)
				} else {
					if err := saveFile(img, screenshotPath); err != nil {
						log.Printf("Error saving screenshot: %v", err)
					}
				}

				// Save the extracted data to JSON
				saveErr := s.saveProfileData(profileData, jsonOutputFile)
				if saveErr != nil {
					log.Printf("Error saving profile data: %v", saveErr)
				}

				instanceSuccess = true
				nitterSuccess = true
				dataExtracted = true
				return
			}

			// If direct extraction failed, take a screenshot and analyze it
			log.Println("Could not extract data directly, taking screenshot for analysis...")

			// Scroll down a bit to ensure profile information is in view
			scrollErr := nitterPage.Mouse.Scroll(0, 200, 8)
			if scrollErr != nil {
				log.Printf("Error scrolling page: %v", scrollErr)
			}

			// Take screenshot and save
			img, screenshotErr := nitterPage.Screenshot(false, &proto.PageCaptureScreenshot{})
			if screenshotErr != nil {
				log.Printf("Error taking screenshot: %v", screenshotErr)
				return
			}

			if err := saveFile(img, screenshotPath); err != nil {
				log.Printf("Error saving screenshot: %v", err)
				return
			}

			log.Println("Screenshot saved successfully")
			instanceSuccess = true
			nitterSuccess = true

			// Try to analyze the screenshot
			visionProfileData, err := s.analyzeScreenshot(screenshotPath)
			if err == nil && (visionProfileData.FollowerCount != "" || visionProfileData.Npub != "") {
				log.Println("Successfully extracted data from screenshot")
				log.Printf("Extracted data: npub=%v, follower_count=%v\n",
					visionProfileData.Npub, visionProfileData.FollowerCount)

				// Save the extracted data to JSON
				saveErr := s.saveProfileData(visionProfileData, jsonOutputFile)
				if saveErr != nil {
					log.Printf("Error saving profile data: %v", saveErr)
				}
				dataExtracted = true
			} else {
				log.Printf("Failed to extract data from screenshot: %v\n", err)
			}
		}()

		// If we succeeded, break the loop
		if nitterSuccess && dataExtracted {
			break
		}

		// Add a small delay between attempts
		if attempts < 2 {
			time.Sleep(2 * time.Second)
		}
	}

	if !nitterSuccess {
		return false, false, lastError
	}

	return nitterSuccess, dataExtracted, nil
}
