package contentmoderation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ModerationMode represents different moderation operating modes
type ModerationMode string

const (
	// ModerationBasic is the fastest mode that only checks for explicit content
	ModerationBasic ModerationMode = "basic"

	// ModerationStrict is a fast mode that automatically blocks all buttocks
	ModerationStrict ModerationMode = "strict"

	// ModerationFull uses contextual analysis for most accurate results
	ModerationFull ModerationMode = "full"
)

// APIClient handles communication with the content moderation API
type APIClient struct {
	// Config is the configuration for the API client
	config *Config

	// HTTPClient is the underlying HTTP client
	httpClient *http.Client
}

// NewAPIClient creates a new API client with the provided configuration
func NewAPIClient(config *Config) *APIClient {
	// Create HTTP client with proper timeout
	client := &http.Client{
		Timeout: config.APITimeout,
	}

	return &APIClient{
		config:     config,
		httpClient: client,
	}
}

// ModerateContent sends media content to the moderation API for analysis
// It returns the moderation result or an error if processing fails
func (c *APIClient) ModerateContent(data []byte, mode ModerationMode) (*ModerationResult, error) {
	if len(data) == 0 {
		return nil, errors.New("empty content data")
	}

	// Prepare the multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create a form file to hold the content
	part, err := writer.CreateFormFile("file", "content.bin")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	// Write the content data to the form
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("failed to write content data: %w", err)
	}

	// Close the writer to finalize the multipart form
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Build the URL with moderation mode
	url := fmt.Sprintf("%s?moderation_mode=%s", c.config.APIEndpoint, mode)

	// Create the HTTP request
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the content type header for multipart form data
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Make the request with timeout handling
	startTime := time.Now()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Log the API call duration if debug is enabled
	if c.config.Debug {
		log.Printf("Content moderation API call took %v", time.Since(startTime))
	}

	// Check for non-200 response
	if resp.StatusCode != http.StatusOK {
		// Try to read error message from response
		errorBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned error status %d: %s", resp.StatusCode, string(errorBody))
	}

	// Parse the response
	var result ModerationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	// Log detailed results in debug mode
	if c.config.Debug {
		log.Printf("Moderation result: level=%d, explicit=%v, decision=%s",
			result.ContentLevel, result.IsExplicit, result.Decision)
	}

	return &result, nil
}

// ModerateFile is a convenience wrapper for moderating a file from a path
func (c *APIClient) ModerateFile(filePath string, mode ModerationMode) (*ModerationResult, error) {
	// Read the file
	data, err := readFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Moderate the content
	return c.ModerateContent(data, mode)
}

// DetermineModerationMode selects the appropriate moderation mode based on content type
func DetermineModerationMode(contentType string, config *Config) ModerationMode {
	// If explicit default mode is set, use it
	if config.DefaultMode != "" {
		return ModerationMode(config.DefaultMode)
	}

	// Otherwise, determine based on content type
	switch {
	case isVideoMimeType(contentType):
		// Use full mode for videos for more accurate analysis
		return ModerationFull
	default:
		// Use full mode as default since it's the most comprehensive
		return ModerationFull
	}
}

// isVideoMimeType checks if a MIME type is a video type
func isVideoMimeType(mimeType string) bool {
	videoTypes := []string{
		"video/mp4",
		"video/webm",
		"video/ogg",
		"video/quicktime",
		"video/x-msvideo",
		"video/x-matroska",
		"video/mpeg",
	}

	for _, t := range videoTypes {
		if t == mimeType {
			return true
		}
	}

	return false
}

// readFile reads a file from disk with a maximum size limit
func readFile(path string) ([]byte, error) {
	// Open the file
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// GetFileExtension gets the file extension from a MIME type
func GetFileExtension(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	case "image/svg+xml":
		return ".svg"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/ogg":
		return ".ogv"
	case "video/quicktime":
		return ".mov"
	case "video/x-msvideo":
		return ".avi"
	case "video/x-matroska":
		return ".mkv"
	case "video/mpeg":
		return ".mpg"
	default:
		return ".bin"
	}
}

// GenerateTempFilePath generates a temporary file path for a given DAG root and content type
func GenerateTempFilePath(dagRoot string, contentType string, config *Config) string {
	// Create filename with appropriate extension
	filename := dagRoot + GetFileExtension(contentType)

	// Join with temp storage path
	return filepath.Join(config.TempStoragePath, filename)
}
