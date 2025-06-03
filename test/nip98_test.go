package test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/web"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/stretchr/testify/assert"
)

func TestNIP98Middleware(t *testing.T) {
	app := fiber.New()

	// Test endpoint with NIP-98 middleware
	app.Put("/test", web.NIP98Middleware(), func(c *fiber.Ctx) error {
		pubkey, err := web.GetNIP98Pubkey(c)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to get pubkey",
			})
		}
		return c.JSON(fiber.Map{
			"pubkey": pubkey,
			"body":   string(c.Body()),
		})
	})

	t.Run("Valid NIP-98 Auth", func(t *testing.T) {
		// Generate a test keypair
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)

		// Create test payload
		payload := []byte("test data")
		hash := sha256.Sum256(payload)
		payloadHash := hex.EncodeToString(hash[:])

		// Create NIP-98 event
		event := nostr.Event{
			PubKey:    pk,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      27235,
			Tags: nostr.Tags{
				{"u", "http://example.com/test"},
				{"method", "PUT"},
				{"payload", payloadHash},
			},
			Content: "",
		}
		event.Sign(sk)

		// Encode event for Authorization header
		eventJSON, _ := json.Marshal(event)
		authHeader := "Nostr " + base64.StdEncoding.EncodeToString(eventJSON)

		// Make request
		req := httptest.NewRequest("PUT", "/test", bytes.NewReader(payload))
		req.Header.Set("Authorization", authHeader)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Check response
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, pk, result["pubkey"])
		assert.Equal(t, string(payload), result["body"])
	})

	t.Run("Missing Authorization Header", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test", nil)
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Invalid Authorization Scheme", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test", nil)
		req.Header.Set("Authorization", "Bearer token")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Invalid Event Kind", func(t *testing.T) {
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)

		event := nostr.Event{
			PubKey:    pk,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      1, // Wrong kind
			Tags: nostr.Tags{
				{"u", "http://example.com/test"},
				{"method", "PUT"},
			},
			Content: "",
		}
		event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		authHeader := "Nostr " + base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest("PUT", "/test", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Expired Event", func(t *testing.T) {
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)

		// Create event with old timestamp
		event := nostr.Event{
			PubKey:    pk,
			CreatedAt: nostr.Timestamp(time.Now().Add(-2 * time.Minute).Unix()),
			Kind:      27235,
			Tags: nostr.Tags{
				{"u", "http://example.com/test"},
				{"method", "PUT"},
			},
			Content: "",
		}
		event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		authHeader := "Nostr " + base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest("PUT", "/test", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Method Mismatch", func(t *testing.T) {
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)

		event := nostr.Event{
			PubKey:    pk,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      27235,
			Tags: nostr.Tags{
				{"u", "http://example.com/test"},
				{"method", "GET"}, // Wrong method
			},
			Content: "",
		}
		event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		authHeader := "Nostr " + base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest("PUT", "/test", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Payload Hash Mismatch", func(t *testing.T) {
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)

		payload := []byte("test data")

		event := nostr.Event{
			PubKey:    pk,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      27235,
			Tags: nostr.Tags{
				{"u", "http://example.com/test"},
				{"method", "PUT"},
				{"payload", "wronghash"}, // Wrong hash
			},
			Content: "",
		}
		event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		authHeader := "Nostr " + base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest("PUT", "/test", bytes.NewReader(payload))
		req.Header.Set("Authorization", authHeader)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestNIP98ProtectedHandler(t *testing.T) {
	app := fiber.New()

	// Test endpoint using NIP98ProtectedHandler helper
	handler := func(c *fiber.Ctx) error {
		pubkey, _ := web.GetNIP98Pubkey(c)
		return c.JSON(fiber.Map{"pubkey": pubkey})
	}

	// Use the middleware directly since NIP98ProtectedHandler is a wrapper
	app.Put("/protected", web.NIP98Middleware(), handler)

	t.Run("Valid Auth with Middleware", func(t *testing.T) {
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)

		event := nostr.Event{
			PubKey:    pk,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      27235,
			Tags: nostr.Tags{
				{"u", "http://example.com/protected"},
				{"method", "PUT"},
			},
			Content: "",
		}
		event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		authHeader := "Nostr " + base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest("PUT", "/protected", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.Equal(t, pk, result["pubkey"])
	})
}

func TestNIP98ConfigurableTimeWindow(t *testing.T) {
	app := fiber.New()

	// Test with custom time window (5 seconds)
	config := web.NIP98Config{
		TimeWindow: 5 * time.Second,
	}

	app.Put("/test", web.NIP98Middleware(config), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	t.Run("Event Outside Custom Time Window", func(t *testing.T) {
		sk := nostr.GeneratePrivateKey()
		pk, _ := nostr.GetPublicKey(sk)

		// Create event 10 seconds old (outside 5 second window)
		event := nostr.Event{
			PubKey:    pk,
			CreatedAt: nostr.Timestamp(time.Now().Add(-10 * time.Second).Unix()),
			Kind:      27235,
			Tags: nostr.Tags{
				{"u", "http://example.com/test"},
				{"method", "PUT"},
			},
			Content: "",
		}
		event.Sign(sk)

		eventJSON, _ := json.Marshal(event)
		authHeader := "Nostr " + base64.StdEncoding.EncodeToString(eventJSON)

		req := httptest.NewRequest("PUT", "/test", nil)
		req.Header.Set("Authorization", authHeader)

		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}
