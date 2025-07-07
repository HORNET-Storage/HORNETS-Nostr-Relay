package auth

import (
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
)

// Refactored loginUser function
func LoginUser(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Login request received")
	var loginPayload types.LoginPayload

	if err := c.BodyParser(&loginPayload); err != nil {
		logging.Infof("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Find the user by npub
	user, err := store.GetStatsStore().FindUserByNpub(loginPayload.Npub)
	if err != nil {
		logging.Infof("User not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid npub or password",
		})
	}

	// Compare passwords
	if err := store.GetStatsStore().ComparePasswords(user.Pass, loginPayload.Password); err != nil {
		logging.Infof("Invalid password for user %s: %v", user.Npub, err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid npub or password",
		})
	}

	// Try to create and save challenge with retries
	maxRetries := 3
	var challenge, hash string
	var event *nostr.Event
	var saveErr error

	for i := 0; i < maxRetries; i++ {
		challenge, hash, err = generateChallenge()
		if err != nil {
			logging.Infof("Error generating challenge: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error",
			})
		}

		event = &nostr.Event{
			PubKey:    user.Npub,
			CreatedAt: nostr.Timestamp(time.Now().Unix()),
			Kind:      1,
			Tags:      nostr.Tags{},
			Content:   challenge,
		}

		userChallenge := types.UserChallenge{
			UserID:    user.ID,
			Npub:      user.Npub,
			Challenge: challenge,
			Hash:      hash,
		}

		saveErr = store.GetStatsStore().SaveUserChallenge(&userChallenge)
		if saveErr == nil {
			break
		}

		if i == maxRetries-1 {
			logging.Infof("Failed to save challenge after %d attempts: %v", maxRetries, saveErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error",
			})
		}

		logging.Infof("Challenge collision occurred, retrying (%d/%d)", i+1, maxRetries)
	}

	logging.Infof("Login challenge created for user %s", user.Npub)

	return c.JSON(fiber.Map{
		"event": event,
	})
}
