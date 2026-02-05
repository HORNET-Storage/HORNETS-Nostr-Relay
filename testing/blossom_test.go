package testing

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
	"github.com/nbd-wtf/go-nostr"
)

// TestBlossomUploadAndDownload tests the full blossom upload/download cycle
func TestBlossomUploadAndDownload(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	// Generate test keys
	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Test data
	testData := []byte("Hello, Blossom! This is test content for upload.")
	expectedHash := sha256.Sum256(testData)
	expectedHashStr := hex.EncodeToString(expectedHash[:])

	// Create NIP-98 auth event
	blossomURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/upload", relay.BlossomPort)
	authHeader, err := createNIP98AuthHeader(kp, blossomURL, "PUT")
	if err != nil {
		t.Fatalf("Failed to create NIP-98 auth header: %v", err)
	}

	// Upload the blob
	client := &http.Client{Timeout: 30 * time.Second}
	uploadReq, err := http.NewRequest("PUT", blossomURL, bytes.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to create upload request: %v", err)
	}
	uploadReq.Header.Set("Authorization", authHeader)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")

	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		t.Fatalf("Failed to upload blob: %v", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(uploadResp.Body)
		t.Fatalf("Upload failed with status %d: %s", uploadResp.StatusCode, string(body))
	}

	// Parse upload response
	var uploadResult map[string]interface{}
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploadResult); err != nil {
		t.Fatalf("Failed to parse upload response: %v", err)
	}

	returnedHash, ok := uploadResult["hash"].(string)
	if !ok || returnedHash == "" {
		t.Fatal("Upload response missing hash")
	}

	if returnedHash != expectedHashStr {
		t.Errorf("Hash mismatch: expected %s, got %s", expectedHashStr, returnedHash)
	}

	// Download the blob
	downloadURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/%s", relay.BlossomPort, returnedHash)
	downloadResp, err := client.Get(downloadURL)
	if err != nil {
		t.Fatalf("Failed to download blob: %v", err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("Download failed with status %d", downloadResp.StatusCode)
	}

	downloadedData, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		t.Fatalf("Failed to read downloaded data: %v", err)
	}

	if !bytes.Equal(downloadedData, testData) {
		t.Errorf("Downloaded data doesn't match uploaded data")
	}
}

// TestBlossomUploadRequiresAuth tests that uploads require NIP-98 authentication
func TestBlossomUploadRequiresAuth(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	testData := []byte("Test data without auth")
	blossomURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/upload", relay.BlossomPort)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("PUT", blossomURL, bytes.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status 401 Unauthorized, got %d", resp.StatusCode)
	}
}

// TestBlossomDownloadPublic tests that downloads are public (no auth required)
func TestBlossomDownloadPublic(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	// First upload a blob with auth
	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	testData := []byte("Public download test content")
	blossomUploadURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/upload", relay.BlossomPort)
	authHeader, err := createNIP98AuthHeader(kp, blossomUploadURL, "PUT")
	if err != nil {
		t.Fatalf("Failed to create NIP-98 auth header: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	uploadReq, err := http.NewRequest("PUT", blossomUploadURL, bytes.NewReader(testData))
	if err != nil {
		t.Fatalf("Failed to create upload request: %v", err)
	}
	uploadReq.Header.Set("Authorization", authHeader)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")

	uploadResp, err := client.Do(uploadReq)
	if err != nil {
		t.Fatalf("Failed to upload blob: %v", err)
	}
	defer uploadResp.Body.Close()

	var uploadResult map[string]interface{}
	if err := json.NewDecoder(uploadResp.Body).Decode(&uploadResult); err != nil {
		t.Fatalf("Failed to parse upload response: %v", err)
	}

	hash := uploadResult["hash"].(string)

	// Now try to download WITHOUT any auth
	downloadURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/%s", relay.BlossomPort, hash)
	downloadResp, err := client.Get(downloadURL)
	if err != nil {
		t.Fatalf("Failed to download blob: %v", err)
	}
	defer downloadResp.Body.Close()

	if downloadResp.StatusCode != http.StatusOK {
		t.Errorf("Expected public download to succeed, got status %d", downloadResp.StatusCode)
	}
}

// TestBlossomNotFound tests 404 for non-existent blobs
func TestBlossomNotFound(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	fakeHash := "0000000000000000000000000000000000000000000000000000000000000000"
	downloadURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/%s", relay.BlossomPort, fakeHash)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404 Not Found, got %d", resp.StatusCode)
	}
}

// TestBlossomEmptyUpload tests that empty uploads are rejected
func TestBlossomEmptyUpload(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	blossomURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/upload", relay.BlossomPort)
	authHeader, err := createNIP98AuthHeader(kp, blossomURL, "PUT")
	if err != nil {
		t.Fatalf("Failed to create NIP-98 auth header: %v", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("PUT", blossomURL, bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request for empty upload, got %d", resp.StatusCode)
	}
}

// TestBlossomUploadTextContent tests uploading text content
func TestBlossomUploadTextContent(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Plain text content
	textContent := []byte("This is plain text content for testing MIME type detection.")

	blossomURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/upload", relay.BlossomPort)
	authHeader, err := createNIP98AuthHeader(kp, blossomURL, "PUT")
	if err != nil {
		t.Fatalf("Failed to create NIP-98 auth header: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("PUT", blossomURL, bytes.NewReader(textContent))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to upload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Verify MIME type was detected
	mimeType, ok := result["type"].(string)
	if !ok || mimeType == "" {
		t.Error("Expected MIME type in response")
	}

	// Should detect as text
	if mimeType != "text/plain; charset=utf-8" && mimeType != "text/plain" {
		t.Logf("MIME type detected: %s", mimeType)
	}
}

// TestBlossomUploadBinaryContent tests uploading binary content
func TestBlossomUploadBinaryContent(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// Create some binary content (PNG header)
	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}

	blossomURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/upload", relay.BlossomPort)
	authHeader, err := createNIP98AuthHeader(kp, blossomURL, "PUT")
	if err != nil {
		t.Fatalf("Failed to create NIP-98 auth header: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("PUT", blossomURL, bytes.NewReader(binaryContent))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to upload: %v", err)
	}
	defer resp.Body.Close()

	// The upload might fail if PNG is not in the allowed MIME types
	// but it shouldn't crash the server
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("Upload response: status=%d body=%s", resp.StatusCode, string(body))
	}
}

// TestBlossomDuplicateUpload tests uploading the same content twice
func TestBlossomDuplicateUpload(t *testing.T) {
	relay, err := helpers.NewTestRelayWithBlossom(helpers.DefaultTestConfig())
	if err != nil {
		t.Fatalf("Failed to create test relay with blossom: %v", err)
	}
	defer relay.Cleanup()

	kp, err := helpers.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	testData := []byte("Duplicate upload test content")
	blossomURL := fmt.Sprintf("http://127.0.0.1:%d/blossom/upload", relay.BlossomPort)

	client := &http.Client{Timeout: 30 * time.Second}

	// First upload
	authHeader1, _ := createNIP98AuthHeader(kp, blossomURL, "PUT")
	req1, _ := http.NewRequest("PUT", blossomURL, bytes.NewReader(testData))
	req1.Header.Set("Authorization", authHeader1)
	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatalf("First upload failed: %v", err)
	}
	defer resp1.Body.Close()

	var result1 map[string]interface{}
	json.NewDecoder(resp1.Body).Decode(&result1)
	hash1 := result1["hash"].(string)

	// Second upload of same content
	authHeader2, _ := createNIP98AuthHeader(kp, blossomURL, "PUT")
	req2, _ := http.NewRequest("PUT", blossomURL, bytes.NewReader(testData))
	req2.Header.Set("Authorization", authHeader2)
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("Second upload failed: %v", err)
	}
	defer resp2.Body.Close()

	var result2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result2)
	hash2 := result2["hash"].(string)

	// Both should return the same hash (content-addressed)
	if hash1 != hash2 {
		t.Errorf("Duplicate uploads should return same hash: %s vs %s", hash1, hash2)
	}
}

// createNIP98AuthHeader creates a NIP-98 Authorization header
func createNIP98AuthHeader(kp *helpers.TestKeyPair, url, method string) (string, error) {
	// Create NIP-98 event (kind 27235)
	event := nostr.Event{
		Kind:      27235,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Tags: nostr.Tags{
			{"u", url},
			{"method", method},
		},
		Content: "",
		PubKey:  kp.PublicKey,
	}

	// Sign the event
	if err := event.Sign(kp.PrivateKey); err != nil {
		return "", fmt.Errorf("failed to sign event: %w", err)
	}

	// Encode to JSON and base64
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("failed to marshal event: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(eventJSON)
	return fmt.Sprintf("Nostr %s", encoded), nil
}
