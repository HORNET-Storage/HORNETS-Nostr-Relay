package wallet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
)

// WalletProxyManager manages wallet JWT token and proxy requests
type WalletProxyManager struct {
	walletBaseURL string
	jwtToken      string
	tokenExpiry   time.Time
}

var proxyManager *WalletProxyManager

func init() {
	proxyManager = &WalletProxyManager{}
}

// getWalletBaseURL gets the wallet URL from config (lazy loading)
func (pm *WalletProxyManager) getWalletBaseURL() string {
	if pm.walletBaseURL == "" {
		pm.walletBaseURL = config.GetExternalURL("wallet")
		logging.Info("Loaded wallet URL from config", map[string]interface{}{
			"wallet_url": pm.walletBaseURL,
		})
	}
	return pm.walletBaseURL
}

// ChallengeRequest represents the challenge request to wallet
type ChallengeRequest struct {
	Content string `json:"content"`
}

// VerifyRequest represents the verify request to wallet
type VerifyRequest struct {
	Challenge   string      `json:"challenge"`
	Signature   string      `json:"signature"`
	MessageHash string      `json:"messageHash"`
	Event       interface{} `json:"event"`
}

// VerifyResponse represents the response from wallet verification
type VerifyResponse struct {
	Token string `json:"token"`
}

// HealthResponse represents the wallet health check response
type HealthResponse struct {
	Status       string `json:"status"`
	Timestamp    string `json:"timestamp"`
	WalletLocked bool   `json:"wallet_locked"`
	ChainSynced  bool   `json:"chain_synced"`
	PeerCount    int    `json:"peer_count"`
}

// CalcTxSizeRequest represents the calculate transaction size request
type CalcTxSizeRequest struct {
	RecipientAddress string `json:"recipient_address"`
	SpendAmount      int64  `json:"spend_amount"`
	PriorityRate     int    `json:"priority_rate"`
}

// CalcTxSizeResponse represents the response from calculate transaction size
type CalcTxSizeResponse struct {
	TxSize int `json:"txSize"`
}

// TransactionRequest represents the transaction request
type TransactionRequest struct {
	Choice           int    `json:"choice"`
	RecipientAddress string `json:"recipient_address,omitempty"`
	SpendAmount      int64  `json:"spend_amount,omitempty"`
	PriorityRate     int    `json:"priority_rate,omitempty"`
	EnableRBF        bool   `json:"enable_rbf,omitempty"`
	OriginalTxID     string `json:"original_tx_id,omitempty"`
	NewFeeRate       int    `json:"new_fee_rate,omitempty"`
}

// TransactionResponse represents the response from transaction endpoint
type TransactionResponse struct {
	Status  string `json:"status"`
	TxID    string `json:"txid"`
	Message string `json:"message"`
}

// HandleChallenge proxies the challenge request to the wallet
func HandleChallenge(c *fiber.Ctx) error {
	walletURL := proxyManager.getWalletBaseURL()
	
	logging.Info("HandleChallenge called", map[string]interface{}{
		"wallet_base_url": walletURL,
	})
	
	if walletURL == "" {
		logging.Error("Wallet service not configured", map[string]interface{}{
			"wallet_base_url": walletURL,
		})
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Wallet service not configured",
		})
	}

	logging.Info("Proxying challenge request to wallet", map[string]interface{}{
		"wallet_url": fmt.Sprintf("%s/challenge", walletURL),
	})

	// Make request to wallet (no authentication required)
	resp, err := http.Get(fmt.Sprintf("%s/challenge", walletURL))
	if err != nil {
		logging.Error("Failed to connect to wallet service", map[string]interface{}{
			"error":      err.Error(),
			"wallet_url": walletURL,
		})
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Failed to connect to wallet service",
		})
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read wallet response",
		})
	}

	logging.Info("Wallet challenge response", map[string]interface{}{
		"status_code": resp.StatusCode,
		"body_length": len(body),
	})

	// Forward the exact response
	c.Set("Content-Type", "application/json")
	return c.Status(resp.StatusCode).Send(body)
}

// HandleVerify proxies the verify request to the wallet and stores the JWT token
func HandleVerify(c *fiber.Ctx) error {
	walletURL := proxyManager.getWalletBaseURL()
	
	if walletURL == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Wallet service not configured",
		})
	}

	// Get request body
	body := c.Body()

	// Make request to wallet
	resp, err := http.Post(
		fmt.Sprintf("%s/verify", walletURL),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		logging.Error("Failed to connect to wallet service", map[string]interface{}{
			"error": err.Error(),
		})
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Failed to connect to wallet service",
		})
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read wallet response",
		})
	}

	// If successful, extract and store the JWT token
	if resp.StatusCode == http.StatusOK {
		var verifyResp VerifyResponse
		if err := json.Unmarshal(respBody, &verifyResp); err == nil && verifyResp.Token != "" {
			proxyManager.jwtToken = verifyResp.Token
			// Set token expiry to 1 hour from now (adjust as needed)
			proxyManager.tokenExpiry = time.Now().Add(time.Hour)

			logging.Info("Wallet JWT token stored successfully")
		}
	}

	// Forward the exact response
	c.Set("Content-Type", "application/json")
	return c.Status(resp.StatusCode).Send(respBody)
}

// makeAuthenticatedRequest makes a request to the wallet with JWT authentication and handles 401 retries
func (pm *WalletProxyManager) makeAuthenticatedRequest(method, endpoint string, body []byte) (*http.Response, error) {
	makeRequest := func(token string) (*http.Response, error) {
		walletURL := pm.getWalletBaseURL()
		url := fmt.Sprintf("%s%s", walletURL, endpoint)

		var req *http.Request
		var err error

		if body != nil {
			req, err = http.NewRequest(method, url, bytes.NewReader(body))
		} else {
			req, err = http.NewRequest(method, url, nil)
		}

		if err != nil {
			return nil, err
		}

		if token != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 30 * time.Second}
		return client.Do(req)
	}

	// First attempt with current token
	resp, err := makeRequest(pm.jwtToken)
	if err != nil {
		return nil, err
	}

	// If we get 401 and have a token, it might be expired - try to re-authenticate
	if resp.StatusCode == 401 && pm.jwtToken != "" {
		resp.Body.Close()

		logging.Warn("Wallet returned 401, attempting to re-authenticate")

		// Clear the old token
		pm.jwtToken = ""
		pm.tokenExpiry = time.Time{}

		// For now, return the 401 - frontend will handle re-authentication
		// In a more sophisticated implementation, we could try to re-authenticate here
		return makeRequest("")
	}

	return resp, nil
}

// HandlePanelHealth proxies the panel health check to the wallet
func HandlePanelHealth(c *fiber.Ctx) error {
	walletURL := proxyManager.getWalletBaseURL()
	
	if walletURL == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Wallet service not configured",
		})
	}

	resp, err := proxyManager.makeAuthenticatedRequest("GET", "/panel-health", nil)
	if err != nil {
		logging.Error("Failed to connect to wallet service", map[string]interface{}{
			"error": err.Error(),
		})
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Failed to connect to wallet service",
		})
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read wallet response",
		})
	}

	// Forward the exact response
	c.Set("Content-Type", "application/json")
	return c.Status(resp.StatusCode).Send(body)
}

// HandleCalculateTxSize proxies the calculate transaction size request to the wallet
func HandleCalculateTxSize(c *fiber.Ctx) error {
	walletURL := proxyManager.getWalletBaseURL()
	
	if walletURL == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Wallet service not configured",
		})
	}

	// Get request body
	body := c.Body()

	resp, err := proxyManager.makeAuthenticatedRequest("POST", "/calculate-tx-size", body)
	if err != nil {
		logging.Error("Failed to connect to wallet service", map[string]interface{}{
			"error": err.Error(),
		})
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Failed to connect to wallet service",
		})
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read wallet response",
		})
	}

	// Forward the exact response
	c.Set("Content-Type", "application/json")
	return c.Status(resp.StatusCode).Send(respBody)
}

// HandleTransaction proxies the transaction request to the wallet (both new and RBF)
func HandleTransaction(c *fiber.Ctx) error {
	walletURL := proxyManager.getWalletBaseURL()
	
	if walletURL == "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Wallet service not configured",
		})
	}

	// Get request body
	body := c.Body()

	resp, err := proxyManager.makeAuthenticatedRequest("POST", "/transaction", body)
	if err != nil {
		logging.Error("Failed to connect to wallet service", map[string]interface{}{
			"error": err.Error(),
		})
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Failed to connect to wallet service",
		})
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to read wallet response",
		})
	}

	// Forward the exact response
	c.Set("Content-Type", "application/json")
	return c.Status(resp.StatusCode).Send(respBody)
}
