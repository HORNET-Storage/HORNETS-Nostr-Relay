package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

func ApiKeyMiddleware(c *fiber.Ctx) error {
	apiKey := c.Get("X-API-Key")
	timestamp := c.Get("X-Timestamp")
	signature := c.Get("X-Signature")

	if apiKey == "" || timestamp == "" || signature == "" {
		log.Println("Missing authentication headers")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing authentication headers",
		})
	}

	expectedAPIKey := viper.GetString("wallet_api_key")
	if apiKey != expectedAPIKey {
		log.Println("Invalid API key")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid API key",
		})
	}

	// Verify timestamp (e.g., within last 5 minutes)
	requestTime, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || time.Since(requestTime) > 5*time.Minute {
		log.Println("Invalid or expired timestamp")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired timestamp",
		})
	}

	// Verify signature
	message := apiKey + timestamp + string(c.Request().Body())
	expectedSignature := generateHMAC(message, expectedAPIKey)

	if signature != expectedSignature {
		log.Println("Invalid signature")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid signature",
		})
	}

	return c.Next()
}

func generateHMAC(message, key string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}
