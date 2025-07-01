package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

func VerifyLoginSignature(c *fiber.Ctx, store stores.Store) error {
	log.Println("Verify login signature: Request received")

	// Parse the payload
	var verifyPayload struct {
		Challenge   string      `json:"challenge"`
		Signature   string      `json:"signature"`
		MessageHash string      `json:"messageHash"`
		Event       nostr.Event `json:"event"`
	}

	if err := c.BodyParser(&verifyPayload); err != nil {
		log.Printf("Verify login signature: JSON parsing error: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	// Verify the event's signature
	event := verifyPayload.Event
	log.Printf("Verify login signature: Event received for pubkey: %s", event.PubKey)
	if !verifyEvent(&event) {
		log.Println("Verify login signature: Invalid event signature")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid event signature",
		})
	}

	// Retrieve the user challenge from the store
	userChallenge, err := store.GetStatsStore().GetUserChallenge(event.Content)
	if err != nil {
		log.Printf("Verify login signature: Challenge not found or expired: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid challenge or challenge expired",
		})
	}

	// Check if the challenge has expired
	if time.Since(userChallenge.CreatedAt) > 3*time.Minute {
		log.Println("Verify login signature: Challenge expired")
		if err := store.GetStatsStore().MarkChallengeExpired(&userChallenge); err != nil {
			log.Printf("Verify login signature: Error updating challenge: %v", err)
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Challenge expired",
		})
	}

	// Retrieve the user based on the challenge
	user, err := store.GetStatsStore().GetUserByID(userChallenge.UserID)
	if err != nil {
		log.Printf("Verify login signature: User not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// if err := store.GetStatsStore().DeleteActiveToken(user.ID); err != nil {
	// 	log.Printf("Warning: Failed to delete existing tokens: %v", err)
	// 	// Continue anyway as this is not critical
	// }

	// Generate JWT token
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &types.JWTClaims{
		UserID: user.ID,
		Email:  user.Npub,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(viper.GetString("jwt_secret")))
	if err != nil {
		log.Printf("Verify login signature: Error creating JWT token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error creating token",
		})
	}

	log.Println("Token String: ", tokenString)

	// Store the active token in the database
	activeToken := types.ActiveToken{
		UserID:    user.ID,
		Token:     tokenString,
		ExpiresAt: expirationTime.Format(time.RFC3339),
	}

	log.Println("Active token to be stored: ", activeToken)
	if err := store.GetStatsStore().StoreActiveToken(&activeToken); err != nil {
		log.Printf("Verify login signature: Failed to store active token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error storing token",
		})
	}

	log.Printf("Verify login signature: Successful verification for user ID: %d", user.ID)

	// Respond with the token and user information
	return c.JSON(fiber.Map{
		"token": tokenString,
		"user": fiber.Map{
			"id":   user.ID,
			"npub": user.Npub,
		},
	})
}

func verifyEvent(event *nostr.Event) bool {
	serialized := SerializeEventForID(event)
	match, hash := HashAndCompare(serialized, event.ID)
	signatureBytes, _ := hex.DecodeString(event.Sig)
	cleanSignature, _ := schnorr.ParseSignature(signatureBytes)
	publicSignatureBytes, _ := hex.DecodeString(event.PubKey)

	cleanPublicKey, _ := schnorr.ParsePubKey(publicSignatureBytes)

	isValid := cleanSignature.Verify(hash[:], cleanPublicKey)

	if isValid {
		fmt.Println("Signature is valid from my implementation")
	} else {
		fmt.Println("Signature is invalid from my implementation")
	}

	isValid, err := event.CheckSignature()
	if err != nil {
		log.Println("Error checking signature:", err)
		return false
	}
	if isValid {
		fmt.Println("Signature is valid")
	} else {
		fmt.Println("Signature is invalid")
	}

	if isValid && match {
		return true
	} else {
		return false
	}
}

func SerializeEventForID(event *nostr.Event) []byte {
	// Assuming the Tags and other fields are already correctly filled except ID and Sig
	serialized, err := json.Marshal([]interface{}{
		0,
		event.PubKey,
		event.CreatedAt,
		event.Kind,
		event.Tags,
		event.Content,
	})
	if err != nil {
		panic(err) // Handle error properly in real code
	}

	return serialized
}

func HashAndCompare(data []byte, hash string) (bool, []byte) {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]) == hash, h[:]
}
