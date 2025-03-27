package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModerationService handles communication with the moderation API
type ModerationService struct {
	APIEndpoint string       // URL of the moderation API
	Threshold   float64      // Confidence threshold for moderation
	Mode        string       // Moderation mode (full, fast, etc.)
	Client      *http.Client // HTTP client for API requests
	DownloadDir string       // Directory for temporarily downloading images
	Enabled     bool         // Whether moderation is enabled
}

// NewModerationService creates a new moderation service instance
func NewModerationService(endpoint string, threshold float64, mode string, timeout time.Duration, downloadDir string) *ModerationService {
	// Create download directory if it doesn't exist
	if downloadDir != "" {
		os.MkdirAll(downloadDir, 0755)
	}

	// Make sure the endpoint is properly formatted
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = "http://" + endpoint
	}

	// Default to port 8000 if not specified
	if !strings.Contains(endpoint, ":") {
		if strings.HasPrefix(endpoint, "https") {
			endpoint = endpoint + ":443"
		} else {
			endpoint = endpoint + ":8000"
		}
	}

	// Ensure endpoint points to /moderate
	if !strings.HasSuffix(endpoint, "/moderate") {
		endpoint = strings.TrimSuffix(endpoint, "/") + "/moderate"
	}

	return &ModerationService{
		APIEndpoint: endpoint,
		Threshold:   threshold,
		Mode:        mode,
		Client:      &http.Client{Timeout: timeout},
		DownloadDir: downloadDir,
		Enabled:     true,
	}
}

// ModerateURL sends an image URL to the moderation API
func (s *ModerationService) ModerateURL(imageURL string) (*ModerationResponse, error) {
	if !s.Enabled {
		// Return default "allow" response if moderation is disabled
		return &ModerationResponse{
			Decision:     string(DecisionAllow),
			Explanation:  "Moderation is disabled",
			ContentLevel: 0,
		}, nil
	}

	// For URL-based moderation, we have two options:
	// 1. Download the image first and then moderate it (more reliable)
	// 2. Send the URL directly to the API (depends on API capability)

	// We'll implement option 1 here, as it's more reliable
	// Download the image
	imagePath, err := s.downloadImage(imageURL)
	if err != nil {
		// If download fails, allow the content to avoid false positives
		// but log the error
		fmt.Printf("Warning: Failed to download image for moderation: %v\n", err)
		return &ModerationResponse{
			Decision:       string(DecisionAllow),
			Explanation:    "Failed to download image for moderation",
			ContentLevel:   0,
			Confidence:     0.0,
			Category:       "error",
			ProcessingTime: 0.0,
			ModerationMode: s.Mode,
		}, nil
	}
	defer os.Remove(imagePath) // Clean up the temporary file

	// Moderate the downloaded image
	return s.ModerateFile(imagePath)
}

// ModerateFile sends a local image file to the moderation API
func (s *ModerationService) ModerateFile(filePath string) (*ModerationResponse, error) {
	if !s.Enabled {
		// Return default "allow" response if moderation is disabled
		return &ModerationResponse{
			Decision:     string(DecisionAllow),
			Explanation:  "Moderation is disabled",
			ContentLevel: 0,
		}, nil
	}

	// Verify the file exists and has content
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat image file: %w", err)
	}

	// Check file size - if it's 0 bytes, it's not a valid image
	if fileInfo.Size() == 0 {
		return nil, fmt.Errorf("image file is empty: %s", filePath)
	}

	// Validate file is an image by checking magic bytes
	imgType, err := validateImageFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("file validation failed: %w", err)
	}

	// Get the absolute path to the file for debugging
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath // Fallback to relative path if abs fails
	}

	// Log file information for debugging
	log.Printf("Uploading image: %s (type: %s, size: %d bytes, path: %s)",
		filepath.Base(filePath), imgType, fileInfo.Size(), absPath)

	// Read the entire file into memory
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %w", err)
	}

	// Create a custom boundary for the form - use the same one that worked in testing
	boundary := "----WebKitFormBoundary7MA4YWxkTrZu0gW"

	// Create a buffer to store the request body
	var requestBody bytes.Buffer

	// Get the filename with proper extension
	filename := filepath.Base(filePath)
	if !strings.Contains(filename, ".") {
		// Add extension based on detected type if missing
		filename = filename + "." + imgType
	}

	// Manually construct the multipart form
	// 1. Add the file part with explicit content type
	requestBody.WriteString("--" + boundary + "\r\n")
	requestBody.WriteString(fmt.Sprintf(`Content-Disposition: form-data; name="file"; filename="%s"`, filename) + "\r\n")
	requestBody.WriteString(fmt.Sprintf("Content-Type: image/%s\r\n\r\n", imgType))
	requestBody.Write(fileData)
	requestBody.WriteString("\r\n")

	// 2. Add moderation mode
	requestBody.WriteString("--" + boundary + "\r\n")
	requestBody.WriteString(`Content-Disposition: form-data; name="moderation_mode"` + "\r\n\r\n")
	requestBody.WriteString(s.Mode + "\r\n")

	// 3. Add threshold
	requestBody.WriteString("--" + boundary + "\r\n")
	requestBody.WriteString(`Content-Disposition: form-data; name="threshold"` + "\r\n\r\n")
	requestBody.WriteString(fmt.Sprintf("%f", s.Threshold) + "\r\n")

	// 4. End of form
	requestBody.WriteString("--" + boundary + "--\r\n")

	// Debug the request body size
	log.Printf("Request body size: %d bytes", requestBody.Len())

	// Create and send the request
	requestURL := fmt.Sprintf("%s?moderation_mode=%s&threshold=%f",
		s.APIEndpoint, s.Mode, s.Threshold)

	req, err := http.NewRequest("POST", requestURL, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", fmt.Sprintf("multipart/form-data; boundary=%s", boundary))
	req.Header.Set("Accept", "application/json")

	// Send the request
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned non-OK status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var result ModerationResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	return &result, nil
}

// downloadImage downloads an image from a URL to a temporary file
func (s *ModerationService) downloadImage(imageURL string) (string, error) {
	// Extract filename from URL and add proper extension if missing
	urlBase := filepath.Base(imageURL)
	urlBase = strings.Split(urlBase, "?")[0] // Remove query parameters

	// Make sure we have a valid file extension
	ext := filepath.Ext(urlBase)
	baseName := strings.TrimSuffix(urlBase, ext)

	// If no extension or invalid, try to detect from URL
	if ext == "" || (ext != ".jpg" && ext != ".jpeg" && ext != ".png" &&
		ext != ".gif" && ext != ".webp" && ext != ".svg") {
		// Try to guess extension from last part of URL
		if strings.Contains(imageURL, ".jpg") || strings.Contains(imageURL, ".jpeg") {
			ext = ".jpg"
		} else if strings.Contains(imageURL, ".png") {
			ext = ".png"
		} else if strings.Contains(imageURL, ".gif") {
			ext = ".gif"
		} else {
			// Default to .jpg if we can't determine
			ext = ".jpg"
		}
	}

	// Create a properly named temporary file
	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), baseName, ext)
	localPath := filepath.Join(s.DownloadDir, filename)

	log.Printf("Downloading image %s to %s", imageURL, localPath)

	// Create a file to save the image
	file, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download the image
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	// Check if download was successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Log content type for debugging
	contentType := resp.Header.Get("Content-Type")
	log.Printf("Image content type from server: %s", contentType)

	// Save the image to file
	size, err := io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	log.Printf("Successfully downloaded %d bytes to %s", size, localPath)

	return localPath, nil
}

// validateImageFile checks if a file is a valid image by examining its magic bytes
// returns the image type as a string (jpg, png, gif, etc.)
func validateImageFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for validation: %w", err)
	}
	defer file.Close()

	// Read the first few bytes to identify file type
	header := make([]byte, 12)
	_, err = file.Read(header)
	if err != nil {
		return "", fmt.Errorf("failed to read file header: %w", err)
	}

	// Check for common image format signatures
	if bytes.HasPrefix(header, []byte{0xFF, 0xD8, 0xFF}) {
		return "jpg", nil // JPEG
	} else if bytes.HasPrefix(header, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return "png", nil // PNG
	} else if bytes.HasPrefix(header, []byte{0x47, 0x49, 0x46, 0x38}) {
		return "gif", nil // GIF
	} else if bytes.HasPrefix(header, []byte{0x42, 0x4D}) {
		return "bmp", nil // BMP
	} else if bytes.HasPrefix(header, []byte{0x52, 0x49, 0x46, 0x46}) &&
		bytes.Equal(header[8:12], []byte{0x57, 0x45, 0x42, 0x50}) {
		return "webp", nil // WebP
	}

	// If we can't identify it, but it has a known image extension, trust the extension
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" ||
		ext == ".webp" || ext == ".bmp" || ext == ".svg" {
		return strings.TrimPrefix(ext, "."), nil
	}

	// File doesn't seem to be an image
	return "", fmt.Errorf("file does not appear to be a valid image")
}

// IsEnabled returns whether the moderation service is enabled
func (s *ModerationService) IsEnabled() bool {
	return s.Enabled
}

// Enable enables the moderation service
func (s *ModerationService) Enable() {
	s.Enabled = true
}

// Disable disables the moderation service
func (s *ModerationService) Disable() {
	s.Enabled = false
}
