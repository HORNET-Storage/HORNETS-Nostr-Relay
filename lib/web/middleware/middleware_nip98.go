package middleware

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
)

const (
	// NIP98EventKind is the kind for HTTP auth events as per NIP-98
	NIP98EventKind = 27235
	// DefaultTimeWindow is the default time window for event validity (60 seconds)
	DefaultTimeWindow = 60 * time.Second
)

// NIP98Context key for storing validated pubkey in request context
const NIP98PubkeyKey = "nip98_pubkey"

// NIP98Config holds configuration for NIP-98 middleware
type NIP98Config struct {
	TimeWindow time.Duration
}

// DefaultNIP98Config returns default configuration
func DefaultNIP98Config() NIP98Config {
	return NIP98Config{
		TimeWindow: DefaultTimeWindow,
	}
}

// NIP98Middleware validates NIP-98 authorization for HTTP requests
func NIP98Middleware(config ...NIP98Config) fiber.Handler {
	cfg := DefaultNIP98Config()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *fiber.Ctx) error {
		// Extract Authorization header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing Authorization header",
			})
		}

		// Check for "Nostr" scheme
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Nostr" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid Authorization scheme, expected 'Nostr'",
			})
		}

		// Decode base64 event
		eventJSON, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid base64 encoding",
			})
		}

		// Parse Nostr event
		var event nostr.Event
		if err := json.Unmarshal(eventJSON, &event); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid Nostr event format",
			})
		}

		// Validate event
		if err := validateNIP98Event(&event, c, cfg); err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// Store pubkey in context for handlers
		c.Locals(NIP98PubkeyKey, event.PubKey)

		return c.Next()
	}
}

// validateNIP98Event performs all NIP-98 validation checks
func validateNIP98Event(event *nostr.Event, c *fiber.Ctx, cfg NIP98Config) error {
	// 1. Check event kind
	if event.Kind != NIP98EventKind {
		return fmt.Errorf("invalid event kind: expected %d, got %d", NIP98EventKind, event.Kind)
	}

	// 2. Check timestamp
	now := time.Now().Unix()
	if event.CreatedAt.Time().Unix() > now+int64(cfg.TimeWindow.Seconds()) ||
		event.CreatedAt.Time().Unix() < now-int64(cfg.TimeWindow.Seconds()) {
		return fmt.Errorf("event timestamp outside acceptable window")
	}

	// 3. Verify signature
	ok, err := event.CheckSignature()
	if err != nil || !ok {
		return fmt.Errorf("invalid event signature")
	}

	// 4. Check URL tag
	urlTag := event.Tags.GetFirst([]string{"u"})
	if urlTag == nil {
		return fmt.Errorf("missing 'u' tag")
	}

	// Build the full URL from the request
	scheme := "http"
	if c.Protocol() == "https" {
		scheme = "https"
	}
	
	// Check for forwarded protocol headers (for proxies like ngrok)
	if proto := c.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	
	// Additional proxy headers to check
	if scheme == "http" {
		if c.Get("X-Forwarded-Ssl") == "on" || c.Get("X-Forwarded-Protocol") == "https" {
			scheme = "https"
		}
	}
	
	// Debug all headers (can be removed in production)
	// fmt.Printf("NIP-98 Debug Headers: X-Forwarded-Proto=%s, X-Forwarded-Ssl=%s, X-Forwarded-Protocol=%s\n", 
	//	c.Get("X-Forwarded-Proto"), c.Get("X-Forwarded-Ssl"), c.Get("X-Forwarded-Protocol"))
	// fmt.Printf("NIP-98 Debug: c.Protocol()=%s, detected scheme=%s\n", c.Protocol(), scheme)

	// Parse the event URL to get what the client expects
	eventURL, err := url.Parse(urlTag.Value())
	if err != nil {
		return fmt.Errorf("failed to parse event URL")
	}

	// Build the request URL that we expect
	directURL := fmt.Sprintf("%s://%s%s", scheme, c.Hostname(), c.OriginalURL())
	
	// Parse direct URL
	requestURL, err := url.Parse(directURL)
	if err != nil {
		return fmt.Errorf("failed to parse request URL")
	}

	// FLEXIBLE VALIDATION: Handle proxy scenarios
	isValid := false
	
	// Method 1: Exact match
	if requestURL.String() == eventURL.String() {
		isValid = true
	}
	
	// Method 2: Proxy-aware validation
	if !isValid {
		// For proxy scenarios, we need to be more flexible
		// Check if this looks like a proxy setup by examining the event URL
		
		// If event URL has a base path (like /panel) but request doesn't,
		// validate that the endpoint paths match and the host is the same
		if eventURL.Host == requestURL.Host {
			// Extract the endpoint part (last segment)
			eventPathParts := strings.Split(strings.Trim(eventURL.Path, "/"), "/")
			requestPathParts := strings.Split(strings.Trim(requestURL.Path, "/"), "/")
			
			// Check if the request path is a suffix of the event path
			// e.g., event: /panel/blossom/upload, request: /blossom/upload
			if len(eventPathParts) >= len(requestPathParts) {
				requestSuffix := strings.Join(requestPathParts, "/")
				eventSuffix := strings.Join(eventPathParts[len(eventPathParts)-len(requestPathParts):], "/")
				
				if requestSuffix == eventSuffix {
					// The paths match at the endpoint level
					// Now check schemes - be flexible about http/https in proxy scenarios
					if (requestURL.Scheme == eventURL.Scheme) || 
					   (requestURL.Scheme == "http" && eventURL.Scheme == "https") ||
					   (requestURL.Scheme == "https" && eventURL.Scheme == "http") {
						isValid = true
					}
				}
			}
		}
	}
	
	if !isValid {
		return fmt.Errorf("URL mismatch: expected %s, got %s", requestURL.String(), eventURL.String())
	}

	// 5. Check method tag
	methodTag := event.Tags.GetFirst([]string{"method"})
	if methodTag == nil {
		return fmt.Errorf("missing 'method' tag")
	}
	if methodTag.Value() != c.Method() {
		return fmt.Errorf("method mismatch: expected %s, got %s", c.Method(), methodTag.Value())
	}

	// 6. Check payload hash for methods with body
	if c.Method() == "POST" || c.Method() == "PUT" || c.Method() == "PATCH" {
		payloadTag := event.Tags.GetFirst([]string{"payload"})
		if payloadTag != nil {
			// Calculate body hash
			body := c.Body()
			hash := sha256.Sum256(body)
			bodyHash := hex.EncodeToString(hash[:])

			if payloadTag.Value() != bodyHash {
				return fmt.Errorf("payload hash mismatch")
			}

			// Restore body for handler
			c.Request().SetBody(body)
		}
	}

	return nil
}

// GetNIP98Pubkey extracts the validated pubkey from the request context
func GetNIP98Pubkey(c *fiber.Ctx) (string, error) {
	pubkey, ok := c.Locals(NIP98PubkeyKey).(string)
	if !ok {
		return "", fmt.Errorf("no authenticated pubkey found")
	}
	return pubkey, nil
}

// NIP98ProtectedHandler is a helper to create handlers that require NIP-98 auth
func NIP98ProtectedHandler(handler fiber.Handler, config ...NIP98Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Apply the middleware
		middleware := NIP98Middleware(config...)
		if err := middleware(c); err != nil {
			return err
		}
		// If middleware passes, call the handler
		return handler(c)
	}
}
