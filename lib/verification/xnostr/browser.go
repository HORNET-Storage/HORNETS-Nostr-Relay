package xnostr

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// initBrowser initializes the browser for scraping with multiple retries
func (s *Service) initBrowser() {
	// Try initialization with a few retries
	for attempt := 0; attempt < 3; attempt++ {
		log.Printf("Browser initialization attempt %d...", attempt+1)

		// Set up the browser launcher with improved settings
		l := launcher.New().
			Headless(true).
			NoSandbox(true).                   // Add NoSandbox for more compatibility
			Leakless(true).                    // Add Leakless to prevent browser processes from leaking
			Set("disable-web-security", "").   // Reduce security restrictions
			Set("disable-setuid-sandbox", ""). // Disable setuid sandbox for better compatibility
			Set("disable-gpu", "").            // Disable GPU to reduce resource usage
			Set("disable-dev-shm-usage", "").  // Don't use /dev/shm, which can be small in some environments
			Set("disable-infobars", "").       // Disable info bars
			Set("window-size", "1280,1024")    // Set a standard window size

		// Use specified browser path if provided
		if s.browserPath != "" {
			log.Printf("Using configured browser path: %s", s.browserPath)
			l.Bin(s.browserPath)
		} else {
			// Try to auto-detect browser
			path, found := autoDetectBrowser()
			if found {
				log.Printf("Using auto-detected browser: %s", path)
				l.Bin(path)
			} else {
				log.Printf("No suitable browser found, will use default system browser")
			}
		}

		// Check if the browser executable exists before trying to launch it
		if s.browserPath != "" {
			if _, err := os.Stat(s.browserPath); os.IsNotExist(err) {
				log.Printf("Browser executable not found at path: %s. Will try auto-detection.", s.browserPath)
				// Try to auto-detect browser
				path, found := autoDetectBrowser()
				if found {
					log.Printf("Using auto-detected browser: %s", path)
					l.Bin(path)
				} else {
					log.Printf("No suitable browser found, will use default system browser")
				}
			}
		}

		// Set a longer timeout for launching
		launchContext, launchCancel := context.WithTimeout(context.Background(), 60*time.Second)

		// Launch the browser in a goroutine to handle timeout safely
		urlChan := make(chan string, 1)
		errChan := make(chan error, 1)

		go func() {
			url, err := l.Launch()
			if err != nil {
				errChan <- err
				return
			}
			urlChan <- url
		}()

		// Wait for launch to complete or timeout
		var url string
		var launchErr error

		select {
		case url = <-urlChan:
			launchCancel()
			log.Printf("Browser launched successfully")
		case launchErr = <-errChan:
			launchCancel()
			log.Printf("Failed to launch browser: %v. Retrying...", launchErr)
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		case <-launchContext.Done():
			launchCancel()
			log.Printf("Browser launch timed out. Retrying...")
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		}

		// Connect to the browser with explicit timeout
		browser := rod.New().
			ControlURL(url).
			Timeout(60 * time.Second).        // Increase timeout
			SlowMotion(50 * time.Millisecond) // Add a small delay to improve stability

		// Try to connect with proper error handling
		connectContext, connectCancel := context.WithTimeout(context.Background(), 60*time.Second)
		connectErrChan := make(chan error, 1)

		go func() {
			connectErrChan <- browser.Connect()
		}()

		var connectErr error
		select {
		case connectErr = <-connectErrChan:
			if connectErr != nil {
				log.Printf("Failed to connect to browser: %v. Retrying...", connectErr)
				connectCancel()
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
				continue
			}
			connectCancel()
			log.Printf("Connected to browser successfully")
		case <-connectContext.Done():
			connectCancel()
			log.Printf("Browser connection timed out. Retrying...")
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		}

		// Test browser by accessing a simple page
		log.Printf("Testing browser connection...")
		testContext, testCancel := context.WithTimeout(context.Background(), 60*time.Second)

		// Create a test page in a goroutine to handle potential hangs
		pageChannel := make(chan *rod.Page, 1)
		pageErrChannel := make(chan error, 1)

		go func() {
			page, err := browser.Page(proto.TargetCreateTarget{})
			if err != nil {
				pageErrChannel <- err
				return
			}
			pageChannel <- page
		}()

		// Wait for page creation or timeout
		var page *rod.Page
		var pageErr error

		select {
		case page = <-pageChannel:
			log.Printf("Test page created successfully")
		case pageErr = <-pageErrChannel:
			testCancel()
			log.Printf("Failed to create test page: %v. Retrying...", pageErr)
			browser.Close()
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		case <-testContext.Done():
			testCancel()
			log.Printf("Test page creation timed out. Retrying...")
			browser.Close()
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		}

		// Use context with the page
		page = page.Context(testContext)

		// Try to navigate to a simple test page
		navigateErrChan := make(chan error, 1)
		go func() {
			navigateErrChan <- page.Navigate("https://example.com")
		}()

		select {
		case navErr := <-navigateErrChan:
			if navErr != nil {
				testCancel()
				log.Printf("Failed to navigate to test page: %v. Retrying...", navErr)
				browser.Close()
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
				continue
			}
		case <-testContext.Done():
			testCancel()
			log.Printf("Navigation timed out. Retrying...")
			browser.Close()
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		}

		// Wait for the page to load
		loadErrChan := make(chan error, 1)
		go func() {
			loadErrChan <- page.WaitLoad()
		}()

		select {
		case loadErr := <-loadErrChan:
			if loadErr != nil {
				testCancel()
				log.Printf("Failed to load test page: %v. Retrying...", loadErr)
				browser.Close()
				time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
				continue
			}
		case <-testContext.Done():
			testCancel()
			log.Printf("Page load timed out. Retrying...")
			browser.Close()
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		}

		// Check if we can actually interact with the page
		titleErrChan := make(chan error, 1)
		titleChan := make(chan string, 1)

		go func() {
			// Get page info instead of using Title() method
			info, err := page.Info()
			if err != nil {
				titleErrChan <- err
				return
			}
			// Get title from page info
			titleChan <- info.Title
		}()

		select {
		case title := <-titleChan:
			log.Printf("Test page title: %s", title)
			testCancel()
		case titleErr := <-titleErrChan:
			testCancel()
			log.Printf("Failed to get page info: %v. Retrying...", titleErr)
			browser.Close()
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		case <-testContext.Done():
			testCancel()
			log.Printf("Get page info operation timed out. Retrying...")
			browser.Close()
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second) // Exponential backoff
			continue
		}

		// Close the test page
		page.Close()

		// If we reached here, the browser is working
		s.browser = browser
		log.Printf("Browser initialized and tested successfully")
		return
	}

	log.Printf("Failed to initialize browser after maximum attempts")
}

// Note: The createSafePage method has been replaced by createSafePageWithBrowser
// to improve browser reuse and reduce initialization overhead

// searchNitterForNostrKeyTweet searches for tweets with the #MyNostrKey: hashtag and extracts the npub
// Only tries the first (highest priority) Nitter instance to avoid unnecessary processing
func (s *Service) searchNitterForNostrKeyTweet(username string) (string, error) {
	// Get a browser from the pool
	browser := s.GetBrowser()
	if browser == nil {
		return "", fmt.Errorf("browser not initialized")
	}
	defer s.ReleaseBrowser(browser)

	log.Println("Searching for tweets with #MyNostrKey: hashtag...")

	// Get the highest priority Nitter instance
	instance := s.GetNextNitterInstance()
	if instance == nil {
		return "", fmt.Errorf("no available Nitter instances")
	}

	nitterBase := instance.URL

	// Only try the first (highest priority) Nitter instance
	{
		// Construct the search URL
		searchURL := nitterBase + username + "/search?f=tweets&q=%23MyNostrKey%3A"
		log.Printf("Trying search URL: %s\n", searchURL)

		// Create a new page with proper error handling and timeout
		// Using the same browser instance for all pages
		page, cancel, err := s.createSafePageWithBrowser(browser, 20*time.Second)
		if err != nil {
			log.Printf("Error creating page for search: %v", err)
			return "", err
		}

		// Use a try-catch approach with recover to handle potential panics
		var npub string
		var searchSuccess bool
		var pageClosedInFunc bool

		func() {
			defer cancel()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from panic while searching Nitter: %v\n", r)
				}
				if !pageClosedInFunc && page != nil {
					page.Close()
					pageClosedInFunc = true
				}
			}()

			// Navigate to the search URL
			err := page.Navigate(searchURL)
			if err != nil {
				log.Printf("Error navigating to search URL: %v\n", err)
				return
			}

			// Wait for page to load
			err = page.WaitLoad()
			if err != nil {
				log.Printf("Timeout waiting for search page to load: %v\n", err)
				return
			}

			// Wait a bit for any dynamic content to load
			time.Sleep(3 * time.Second)

			// Check if we got search results
			timeline, err := page.Element(".timeline")
			if err != nil || timeline == nil {
				log.Println("No timeline found on search page")
				return
			}

			// Find the first tweet
			tweets, err := page.Elements(".timeline-item")
			if err != nil || len(tweets) == 0 {
				log.Println("No tweets found in search results")
				return
			}

			log.Printf("Found %d tweets in search results\n", len(tweets))

			// Extract the text from each tweet and look for the hashtag
			for i, tweet := range tweets {
				tweetText, err := tweet.Text()
				if err != nil {
					log.Printf("Error extracting text from tweet %d: %v\n", i, err)
					continue
				}

				// Look for the hashtag in the tweet text
				hashtagIndex := strings.Index(strings.ToLower(tweetText), "#mynostrkey:")
				if hashtagIndex >= 0 {
					// Extract the text after the hashtag
					afterHashtag := tweetText[hashtagIndex+len("#mynostrkey:"):]

					// Look for npub1 in the text after the hashtag
					npubIndex := strings.Index(afterHashtag, "npub1")
					if npubIndex >= 0 {
						// Extract the npub
						npubStart := npubIndex + hashtagIndex + len("#mynostrkey:")
						npubEnd := npubStart

						// Extract until we hit a space, newline, or end of string
						for npubEnd < len(tweetText) && npubEnd < npubStart+64 &&
							!strings.ContainsRune(" \n\t\r", rune(tweetText[npubEnd])) &&
							((tweetText[npubEnd] >= 'a' && tweetText[npubEnd] <= 'z') ||
								(tweetText[npubEnd] >= '0' && tweetText[npubEnd] <= '9')) {
							npubEnd++
						}

						if npubEnd > npubStart {
							npub = tweetText[npubStart:npubEnd]
							log.Printf("Found npub in tweet: %s\n", npub)
							searchSuccess = true
							return
						}
					} else {
						// If we didn't find npub1 directly, try to extract any word after the hashtag
						// as it might be the npub without the "npub1" prefix
						words := strings.Fields(afterHashtag)
						if len(words) > 0 {
							potentialNpub := words[0]
							// Check if it looks like a valid npub (alphanumeric and reasonable length)
							if len(potentialNpub) >= 5 && len(potentialNpub) <= 64 {
								// If it doesn't start with npub1, add it
								if !strings.HasPrefix(potentialNpub, "npub1") {
									potentialNpub = "npub1" + potentialNpub
								}
								npub = potentialNpub
								log.Printf("Found potential npub in tweet: %s\n", npub)
								searchSuccess = true
								return
							}
						}
					}
				}
			}
		}()

		// If we found an npub, return it
		if searchSuccess && npub != "" {
			return npub, nil
		}
	}

	// If we didn't find an npub in any of the Nitter instances, return an error
	return "", fmt.Errorf("no tweet with #MyNostrKey: hashtag found")
}

// Helper function to save file
func saveFile(data []byte, filePath string) error {
	return ioutil.WriteFile(filePath, data, 0644)
}

// autoDetectBrowser tries to find a suitable browser on the system
func autoDetectBrowser() (string, bool) {
	// Common browser paths on macOS
	macOSBrowsers := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"/Applications/Firefox.app/Contents/MacOS/firefox",
	}

	// Common browser paths on Linux
	linuxBrowsers := []string{
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/firefox",
	}

	// Check if each browser exists
	for _, browser := range append(macOSBrowsers, linuxBrowsers...) {
		_, err := exec.LookPath(browser)
		if err == nil {
			return browser, true
		}
	}

	return "", false
}
