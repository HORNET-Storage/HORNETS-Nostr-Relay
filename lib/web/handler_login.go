package web

import (
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
)

func loginUser(c *fiber.Ctx) error {
	log.Println("Login request received")
	var loginPayload types.LoginPayload

	if err := c.BodyParser(&loginPayload); err != nil {
		log.Printf("Failed to parse JSON: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	dbPath := viper.GetString("relay_stats_db")
	if dbPath == "" {
		log.Fatal("Database path not found in config")
	}

	// Initialize the Gorm database
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	var user types.User
	if err := db.Where("npub = ?", loginPayload.Npub).First(&user).Error; err != nil {
		log.Printf("User not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid npub or password",
		})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginPayload.Password)); err != nil {
		log.Printf("Invalid password for user %s: %v", user.Npub, err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid npub or password",
		})
	}

	challenge, hash, err := generateChallenge()
	if err != nil {
		log.Printf("Error generating challenge: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal server error",
		})
	}

	event := &nostr.Event{
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
	if err := db.Create(&userChallenge).Error; err != nil {
		log.Printf("Failed to save challenge: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal server error",
		})
	}

	log.Printf("Login challenge created for user %s", user.Npub)

	return c.JSON(fiber.Map{
		"event": event,
	})
}
