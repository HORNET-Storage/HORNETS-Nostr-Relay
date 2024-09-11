package web

import (
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
)

func loginUser(c *fiber.Ctx) error {
	log.Println("Login request received")
	var loginPayload struct {
		types.LoginPayload
		Npub string `json:"npub"`
	}

	if err := c.BodyParser(&loginPayload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	db, err := graviton.InitGorm()
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
		log.Printf("Invalid password: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid email or password",
		})
	}

	challenge, hash, err := generateChallenge()
	if err != nil {
		log.Printf("Error generating challenge: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Generate Nostr event
	event := &nostr.Event{
		PubKey:    user.Npub,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      1, // Example kind
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
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// After successfully generating the token
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &types.JWTClaims{
		UserID: user.ID,
		Email:  user.Npub,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		log.Printf("Error creating JWT token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error creating token",
		})
	}

	// Store the active token
	activeToken := types.ActiveToken{
		UserID:    user.ID,
		Token:     tokenString,
		ExpiresAt: expirationTime,
	}
	if err := db.Create(&activeToken).Error; err != nil {
		log.Printf("Failed to store active token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error storing token",
		})
	}

	return c.JSON(fiber.Map{
		"event": event,
		"token": tokenString,
	})
}
