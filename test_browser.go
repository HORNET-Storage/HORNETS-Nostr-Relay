package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

func main() {
	// Create temp directory
	tempDir := "/tmp/xnostr-verification"
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			log.Printf("Failed to create temp directory %s: %v", tempDir, err)
			tempDir = os.TempDir()
			log.Printf("Falling back to system temp directory: %s", tempDir)
		}
	}

	// Try to find a suitable browser automatically
	browserPath, found := autoDetectBrowser()
	if !found {
		log.Fatalf("No suitable browser found. Browser initialization will fail.")
	}
	log.Printf("Using browser: %s", browserPath)

	// Set up the browser launcher
	l := launcher.New().Headless(true)
	l.Bin(browserPath)

	// Launch the browser with detailed error handling
	url, err := l.Launch()
	if err != nil {
		log.Fatalf("Failed to launch browser: %v", err)
	}

	// Connect to the browser
	browser := rod.New().ControlURL(url).Timeout(15 * time.Second)
	
	// Try to connect with proper error handling
	err = browser.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to browser: %v", err)
	}
	defer browser.Close()

	log.Println("Browser initialized successfully")

	// Try to open a page
	page := browser.MustPage()

	// Try to navigate to a simple test page
	err = page.Navigate("https://example.com")
	if err != nil {
		log.Fatalf("Failed to navigate to test page: %v", err)
	}

	log.Println("Successfully navigated to example.com")

	// Take a screenshot
	screenshotPath := fmt.Sprintf("%s/test_screenshot.png", tempDir)
	img, err := page.Screenshot(false, &proto.PageCaptureScreenshot{})
	if err != nil {
		log.Fatalf("Failed to take screenshot: %v", err)
	}

	// Save the screenshot
	err = ioutil.WriteFile(screenshotPath, img, 0644)
	if err != nil {
		log.Fatalf("Failed to save screenshot: %v", err)
	}

	fmt.Printf("Screenshot saved to %s\n", screenshotPath)
	log.Println("Browser test completed successfully")
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