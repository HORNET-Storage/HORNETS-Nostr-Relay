package image

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
)

// ModerationService handles communication with the moderation API
type ModerationService struct {
	APIEndpoint string       // URL of the moderation API
	Threshold   float64      // Confidence threshold for moderation
	Mode        string       // Moderation mode (full, fast, etc.)
	Client      *http.Client // HTTP client for API requests
	DownloadDir string       // Directory for temporarily downloading media files
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

// ModerateURL sends a media URL to the moderation API
func (s *ModerationService) ModerateURL(mediaURL string) (*ModerationResponse, error) {
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
	// Download the media file
	imagePath, err := s.downloadImage(mediaURL)
	if err != nil {
		// If download fails, allow the content to avoid false positives
		// but log the error
		logging.Infof("Warning: Failed to download image for moderation: %v\n", err)
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
	logging.Infof("Uploading image: %s (type: %s, size: %d bytes, path: %s)",
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
	logging.Infof("Request body size: %d bytes", requestBody.Len())

	// Create and send the request
	requestURL := fmt.Sprintf("%s/moderate?moderation_mode=%s&threshold=%f",
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

// downloadMedia downloads media (image or video) from a URL to a temporary file
func (s *ModerationService) downloadImage(mediaURL string) (string, error) {
	// Extract filename from URL and add proper extension if missing
	urlBase := filepath.Base(mediaURL)
	urlBase = strings.Split(urlBase, "?")[0] // Remove query parameters

	// Make sure we have a valid file extension
	ext := filepath.Ext(urlBase)
	baseName := strings.TrimSuffix(urlBase, ext)

	// Determine if this is likely a video based on URL
	isVideoURL := IsVideo(mediaURL)

	// If no extension or invalid, try to detect from URL
	if ext == "" || (!isValidImageExt(ext) && !isValidVideoExt(ext)) {
		// Try to guess extension from URL
		if isVideoURL {
			// Check for common video extensions in the URL
			if strings.Contains(mediaURL, ".mp4") {
				ext = ".mp4"
			} else if strings.Contains(mediaURL, ".webm") {
				ext = ".webm"
			} else if strings.Contains(mediaURL, ".mov") {
				ext = ".mov"
			} else {
				// Default to .mp4 for videos if we can't determine
				ext = ".mp4"
			}
		} else {
			// Try to guess image extension from URL
			if strings.Contains(mediaURL, ".jpg") || strings.Contains(mediaURL, ".jpeg") {
				ext = ".jpg"
			} else if strings.Contains(mediaURL, ".png") {
				ext = ".png"
			} else if strings.Contains(mediaURL, ".gif") {
				ext = ".gif"
			} else {
				// Default to .jpg if we can't determine
				ext = ".jpg"
			}
		}
	}

	// Create a properly named temporary file
	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), baseName, ext)
	localPath := filepath.Join(s.DownloadDir, filename)

	logging.Infof("Downloading media %s to %s", mediaURL, localPath)

	// Create a file to save the media
	file, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download the media
	resp, err := http.Get(mediaURL)
	if err != nil {
		return "", fmt.Errorf("failed to download media: %w", err)
	}
	defer resp.Body.Close()

	// Check if download was successful
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Log content type for debugging
	contentType := resp.Header.Get("Content-Type")
	logging.Infof("Media content type from server: %s", contentType)

	// Save the media to file
	size, err := io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save media: %w", err)
	}

	logging.Infof("Successfully downloaded %d bytes to %s", size, localPath)

	return localPath, nil
}

// Helper to check for valid image extensions
func isValidImageExt(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" ||
		ext == ".gif" || ext == ".webp" || ext == ".bmp" || ext == ".svg" || ext == ".avif"
}

// Helper to check for valid video extensions
func isValidVideoExt(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".mp4" || ext == ".webm" || ext == ".mov" ||
		ext == ".avi" || ext == ".mkv" || ext == ".m4v" || ext == ".ogv" || ext == ".mpg" || ext == ".mpeg"
}

// validateImageFile checks if a file is a valid media file by examining its magic bytes
// returns the media type as a string (jpg, png, mp4, etc.)
func validateImageFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for validation: %w", err)
	}
	defer file.Close()

	// Read the first several bytes to identify file type
	// Need more bytes for some video formats
	header := make([]byte, 16)
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

	// Check for common video format signatures
	if bytes.HasPrefix(header, []byte{0x00, 0x00, 0x00}) &&
		(bytes.Equal(header[4:8], []byte{0x66, 0x74, 0x79, 0x70}) || // ftyp
			bytes.Equal(header[4:8], []byte{0x6D, 0x6F, 0x6F, 0x76})) { // moov
		return "mp4", nil // MP4 or MOV
	} else if bytes.HasPrefix(header, []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return "webm", nil // WebM or MKV
	} else if bytes.HasPrefix(header, []byte{0x52, 0x49, 0x46, 0x46}) && // RIFF
		bytes.Equal(header[8:12], []byte{0x41, 0x56, 0x49, 0x20}) { // AVI
		return "avi", nil // AVI
	} else if bytes.HasPrefix(header, []byte{0x00, 0x00, 0x01, 0xBA}) ||
		bytes.HasPrefix(header, []byte{0x00, 0x00, 0x01, 0xB3}) {
		return "mpeg", nil // MPEG
	}

	// If we can't identify it by magic bytes, trust the extension
	ext := strings.ToLower(filepath.Ext(filePath))

	// Check for image extensions
	if isValidImageExt(ext) {
		return strings.TrimPrefix(ext, "."), nil
	}

	// Check for video extensions
	if isValidVideoExt(ext) {
		return strings.TrimPrefix(ext, "."), nil
	}

	// File doesn't seem to be a recognized media type
	return "", fmt.Errorf("file does not appear to be a valid media file")
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

// ModerateDisputeURL sends a media URL to the moderation API with dispute-specific parameters
func (s *ModerationService) ModerateDisputeURL(mediaURL string, disputeReason string) (*ModerationResponse, error) {
	if !s.Enabled {
		// Return default "allow" response if moderation is disabled
		return &ModerationResponse{
			Decision:     string(DecisionAllow),
			Explanation:  "Moderation is disabled",
			ContentLevel: 0,
		}, nil
	}

	// Download the media file
	imagePath, err := s.downloadImage(mediaURL)
	if err != nil {
		// If download fails, allow the content to avoid false positives
		logging.Infof("Warning: Failed to download image for dispute moderation: %v\n", err)
		return &ModerationResponse{
			Decision:       string(DecisionAllow),
			Explanation:    "Failed to download image for dispute moderation",
			ContentLevel:   0,
			Confidence:     0.0,
			Category:       "error",
			ProcessingTime: 0.0,
			ModerationMode: s.Mode,
		}, nil
	}
	defer os.Remove(imagePath) // Clean up the temporary file

	// Moderate the downloaded image with dispute-specific parameters
	return s.ModerateDisputeFile(imagePath, disputeReason)
}

// ModerateDisputeFile sends a local image file to the moderation API with dispute-specific parameters
func (s *ModerationService) ModerateDisputeFile(filePath string, disputeReason string) (*ModerationResponse, error) {
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
	logging.Infof("Uploading image for dispute moderation: %s (type: %s, size: %d bytes, path: %s)",
		filepath.Base(filePath), imgType, fileInfo.Size(), absPath)

	// Read the entire file into memory
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image file: %w", err)
	}

	// Create a custom boundary for the form
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

	// 2. Add moderation mode - always use "full" for disputes
	requestBody.WriteString("--" + boundary + "\r\n")
	requestBody.WriteString(`Content-Disposition: form-data; name="moderation_mode"` + "\r\n\r\n")
	requestBody.WriteString("full\r\n")

	// 3. Add threshold - use a lower threshold for disputes (0.35 instead of 0.4)
	requestBody.WriteString("--" + boundary + "\r\n")
	requestBody.WriteString(`Content-Disposition: form-data; name="threshold"` + "\r\n\r\n")
	requestBody.WriteString(fmt.Sprintf("%f", 0.35) + "\r\n")

	// 4. Add dispute reason if provided
	if disputeReason != "" {
		requestBody.WriteString("--" + boundary + "\r\n")
		requestBody.WriteString(`Content-Disposition: form-data; name="dispute_reason"` + "\r\n\r\n")
		requestBody.WriteString(disputeReason + "\r\n")
	}

	// 5. End of form
	requestBody.WriteString("--" + boundary + "--\r\n")

	// Debug the request body size
	logging.Infof("Dispute moderation request body size: %d bytes", requestBody.Len())

	// Create and send the request
	requestURL := fmt.Sprintf("%s/moderate_dispute?moderation_mode=%s&threshold=%f", s.APIEndpoint, s.Mode, s.Threshold)

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
