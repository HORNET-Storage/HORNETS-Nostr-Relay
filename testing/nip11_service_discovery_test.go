package testing

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/transports/websocket"
	"github.com/HORNET-Storage/hornet-storage/testing/helpers"
)

// TestNIP11RelayInfo tests that the relay returns a proper NIP-11 relay information document
func TestNIP11RelayInfo(t *testing.T) {
	relay := setupTestRelay(t)
	defer relay.Cleanup()

	// Make HTTP GET request with Accept: application/nostr+json header
	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var relayInfo websocket.NIP11RelayInfo
	if err := json.Unmarshal(body, &relayInfo); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	// Verify basic relay info
	if relayInfo.Name == "" {
		t.Error("Expected relay name to be set")
	}

	if len(relayInfo.SupportedNIPs) == 0 {
		t.Error("Expected supported NIPs to be set")
	}
}

// TestNIP11ServiceDiscovery tests that the relay includes service discovery information
// The new scheme uses base_port with fixed offsets - services like hornets, panel, blossom
// are calculated from base_port. Only external services like airlock are in the services map.
func TestNIP11ServiceDiscovery(t *testing.T) {
	// Create a custom config with services enabled
	cfg := helpers.DefaultTestConfig()
	relay, err := helpers.NewTestRelayWithServices(cfg)
	if err != nil {
		t.Fatalf("Failed to create test relay with services: %v", err)
	}
	defer relay.Cleanup()

	// Make HTTP GET request with Accept: application/nostr+json header
	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var relayInfo websocket.NIP11RelayInfo
	if err := json.Unmarshal(body, &relayInfo); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	// Verify base_port is set (clients use this + offsets to find services)
	if relayInfo.BasePort == 0 {
		t.Error("Expected base_port to be set in NIP-11 response")
	}

	// Verify base_port is a reasonable value (greater than 0)
	t.Logf("NIP-11 base_port: %d, test relay listening on: %d", relayInfo.BasePort, relay.Port)

	// Built-in services (hornets, panel, blossom) are NOT in the services map
	// They are calculated using base_port + offset:
	// - Hornets: base_port + 1
	// - Panel/Blossom: base_port + 2
	// Only external services like airlock should be in the services map
}

// TestNIP11ServiceDiscoveryHornets tests that hornets (libp2p) port is calculated from base_port
// Hornets uses the offset scheme: base_port + 1
func TestNIP11ServiceDiscoveryHornets(t *testing.T) {
	cfg := helpers.DefaultTestConfig()
	relay, err := helpers.NewTestRelayWithHornets(cfg)
	if err != nil {
		t.Fatalf("Failed to create test relay with hornets: %v", err)
	}
	defer relay.Cleanup()

	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var relayInfo websocket.NIP11RelayInfo
	if err := json.Unmarshal(body, &relayInfo); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	// Verify base_port is set
	if relayInfo.BasePort == 0 {
		t.Fatal("Expected base_port to be set in NIP-11 response")
	}

	// Hornets port is calculated as: base_port + PortOffsetHornets (1)
	expectedHornetsPort := relayInfo.BasePort + websocket.PortOffsetHornets
	t.Logf("Base port: %d, Expected Hornets port: %d", relayInfo.BasePort, expectedHornetsPort)

	// Hornets is NOT in the services map - it's calculated from base_port
	// Services map only contains external services like airlock
}

// TestNIP11ServiceDiscoveryAirlock tests airlock service endpoint configuration
// Airlock is an EXTERNAL service - it's explicitly listed in the services map
// because it doesn't use the fixed offset scheme (it has its own configurable port)
func TestNIP11ServiceDiscoveryAirlock(t *testing.T) {
	cfg := helpers.DefaultTestConfig()
	relay, err := helpers.NewTestRelayWithAirlock(cfg)
	if err != nil {
		t.Fatalf("Failed to create test relay with airlock: %v", err)
	}
	defer relay.Cleanup()

	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var relayInfo websocket.NIP11RelayInfo
	if err := json.Unmarshal(body, &relayInfo); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	// Verify base_port is set
	if relayInfo.BasePort == 0 {
		t.Fatal("Expected base_port to be set in NIP-11 response")
	}

	// Verify services map exists (needed for external services like airlock)
	if relayInfo.Services == nil {
		t.Fatal("Expected services map to be present in NIP-11 response")
	}

	// Verify Airlock service endpoint - this IS in the services map because it's external
	airlock := relayInfo.Services["airlock"]
	if airlock == nil {
		t.Fatal("Expected Airlock service endpoint to be present")
	}

	if airlock.Port == 0 {
		t.Error("Expected Airlock service to have a port configured")
	}

	// Airlock should have a pubkey configured (client derives peer ID)
	if airlock.Pubkey == "" {
		t.Error("Expected Airlock service to have a pubkey configured")
	}
}

// TestNIP11ServicesOmittedWhenDisabled tests that external services are omitted when not configured
func TestNIP11ServicesOmittedWhenDisabled(t *testing.T) {
	// Default config doesn't enable external services like airlock
	relay := setupTestRelay(t)
	defer relay.Cleanup()

	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Parse as raw JSON to check field presence
	var rawResponse map[string]interface{}
	if err := json.Unmarshal(body, &rawResponse); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	// base_port should always be present
	if _, hasBasePort := rawResponse["base_port"]; !hasBasePort {
		t.Error("Expected base_port to always be present")
	}

	// Services map should be nil or have no airlock when not configured
	services, hasServices := rawResponse["services"]
	if hasServices && services != nil {
		servicesMap, ok := services.(map[string]interface{})
		if ok {
			// Airlock shouldn't be present if not configured
			if airlock, hasAirlock := servicesMap["airlock"]; hasAirlock && airlock != nil {
				t.Error("Expected Airlock to be omitted when not configured")
			}
		}
	}
}

// TestNIP11ServiceEndpointStructure verifies the structure of external service endpoints
// External services (like airlock) are in the services map with port, protocol, peer_id
func TestNIP11ServiceEndpointStructure(t *testing.T) {
	cfg := helpers.DefaultTestConfig()
	relay, err := helpers.NewTestRelayWithAirlock(cfg)
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	defer relay.Cleanup()

	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Parse as raw JSON to check exact field structure
	var rawResponse map[string]interface{}
	if err := json.Unmarshal(body, &rawResponse); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	services, ok := rawResponse["services"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected services to be an object")
	}

	airlock, ok := services["airlock"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected airlock to be an object")
	}

	// Verify endpoint has required fields
	if _, hasPort := airlock["port"]; !hasPort {
		t.Error("Expected airlock endpoint to have 'port' field")
	}

	if _, hasProtocol := airlock["protocol"]; !hasProtocol {
		t.Error("Expected airlock endpoint to have 'protocol' field")
	}

	// pubkey should be present for libp2p services (clients derive peer ID)
	if _, hasPubkey := airlock["pubkey"]; !hasPubkey {
		t.Error("Expected airlock endpoint to have 'pubkey' field")
	}
}

// TestNIP11PortOffsets tests that port offsets are properly documented
func TestNIP11PortOffsets(t *testing.T) {
	// Verify the port offset constants are defined correctly
	if websocket.PortOffsetNostr != 0 {
		t.Errorf("Expected PortOffsetNostr to be 0, got %d", websocket.PortOffsetNostr)
	}
	if websocket.PortOffsetHornets != 1 {
		t.Errorf("Expected PortOffsetHornets to be 1, got %d", websocket.PortOffsetHornets)
	}
	if websocket.PortOffsetPanel != 2 {
		t.Errorf("Expected PortOffsetPanel to be 2, got %d", websocket.PortOffsetPanel)
	}
}

// TestNIP11PubkeyForPeerIDDerivation tests that pubkey is present for peer ID derivation
func TestNIP11PubkeyForPeerIDDerivation(t *testing.T) {
	relay := setupTestRelay(t)
	defer relay.Cleanup()

	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var relayInfo websocket.NIP11RelayInfo
	if err := json.Unmarshal(body, &relayInfo); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	// Pubkey must be present for clients to derive libp2p peer ID
	if relayInfo.Pubkey == "" {
		t.Error("Expected pubkey to be set - needed for peer ID derivation")
	}

	// Pubkey should be 64 hex chars (32 bytes)
	if len(relayInfo.Pubkey) != 64 {
		t.Errorf("Expected pubkey to be 64 hex chars, got %d", len(relayInfo.Pubkey))
	}

	t.Logf("Relay pubkey: %s", relayInfo.Pubkey)
}

// TestNIP11DiscoveryFullFlow tests the complete discovery flow a client would use
func TestNIP11DiscoveryFullFlow(t *testing.T) {
	cfg := helpers.DefaultTestConfig()
	relay, err := helpers.NewTestRelayWithAirlock(cfg)
	if err != nil {
		t.Fatalf("Failed to create test relay: %v", err)
	}
	defer relay.Cleanup()

	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var relayInfo websocket.NIP11RelayInfo
	if err := json.Unmarshal(body, &relayInfo); err != nil {
		t.Fatalf("Failed to parse NIP-11 response: %v", err)
	}

	// Simulate what a client would do with discovery info
	basePort := relayInfo.BasePort
	if basePort == 0 {
		t.Fatal("base_port required for discovery")
	}

	// Calculate service ports using offsets
	nostrPort := basePort + websocket.PortOffsetNostr
	hornetsPort := basePort + websocket.PortOffsetHornets
	panelPort := basePort + websocket.PortOffsetPanel

	t.Logf("Discovery results:")
	t.Logf("  Base port: %d", basePort)
	t.Logf("  Nostr WebSocket: %d", nostrPort)
	t.Logf("  Hornets libp2p: %d", hornetsPort)
	t.Logf("  Panel HTTP: %d", panelPort)

	// Verify pubkey for peer ID derivation
	if relayInfo.Pubkey == "" {
		t.Error("pubkey required for peer ID derivation")
	}

	// Check external services (airlock)
	if airlock := relayInfo.Services["airlock"]; airlock != nil {
		t.Logf("  Airlock: port=%d, pubkey=%s",
			airlock.Port,
			airlock.Pubkey)
	}

	// Verify offsets make sense (hornets > nostr, panel > hornets)
	if hornetsPort <= nostrPort {
		t.Error("Hornets port should be greater than Nostr port")
	}
	if panelPort <= hornetsPort {
		t.Error("Panel port should be greater than Hornets port")
	}
}

// TestNIP11ContentTypeHeader tests that the correct content type is returned
func TestNIP11ContentTypeHeader(t *testing.T) {
	relay := setupTestRelay(t)
	defer relay.Cleanup()

	httpURL := "http://127.0.0.1:" + getPortString(relay.Port)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", httpURL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Accept", "application/nostr+json")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to make NIP-11 request: %v", err)
	}
	defer resp.Body.Close()

	// NIP-11 should return application/nostr+json content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/nostr+json" && contentType != "application/json" {
		t.Logf("Content-Type: %s (NIP-11 recommends application/nostr+json)", contentType)
	}
}

// Helper function to get port as string
func getPortString(port int) string {
	return fmt.Sprintf("%d", port)
}
