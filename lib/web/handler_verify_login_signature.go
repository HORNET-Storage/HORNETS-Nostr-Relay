package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/golang-jwt/jwt/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/gofiber/fiber/v2"
)

func verifyLoginSignature(c *fiber.Ctx) error {
	log.Println("Verify login signature: Request received")
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

	event := verifyPayload.Event
	log.Printf("Verify login signature: Event received for pubkey: %s", event.PubKey)

	// Verify event (challenge response)
	if !verifyEvent(&event) {
		log.Println("Verify login signature: Invalid event signature")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid event signature",
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

	var userChallenge types.UserChallenge
	if err := db.Where("challenge = ? AND expired = ?", event.Content, false).First(&userChallenge).Error; err != nil {
		log.Printf("Verify login signature: Challenge not found or expired: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid challenge or challenge expired",
		})
	}

	// Check if the challenge is expired
	if time.Since(userChallenge.CreatedAt) > 3*time.Minute {
		log.Println("Verify login signature: Challenge expired")
		db.Model(&userChallenge).Update("expired", true)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Challenge expired",
		})
	}

	var user types.User
	if err := db.Where("id = ?", userChallenge.UserID).First(&user).Error; err != nil {
		log.Printf("Verify login signature: User not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

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
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		log.Printf("Verify login signature: Error creating JWT token: %v", err)
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
		log.Printf("Verify login signature: Failed to store active token: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error storing token",
		})
	}

	log.Printf("Verify login signature: Successful verification for user ID: %d", user.ID)

	return c.JSON(fiber.Map{
		"token": tokenString,
		"user": fiber.Map{
			"id":   user.ID,
			"npub": user.Npub,
		},
	})
}

func verifyEvent(event *nostr.Event) bool {
	serialized := serializeEventForID(event)
	log.Println("The Event ID is:", event.ID)
	match, hash := HashAndCompare(serialized, event.ID)
	if match {
		fmt.Println("Hash matches ID:", event.ID)
	} else {
		fmt.Println("Hash does not match ID")
	}
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

	log.Println("Event tags: ", event.Tags)

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

func serializeEventForID(event *nostr.Event) []byte {
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

func DeserializePublicKey(serializedKey string) (*secp256k1.PublicKey, error) {
	publicKeyBytes, err := DecodeKey(serializedKey)
	if err != nil {
		return nil, err
	}

	publicKey, err := schnorr.ParsePubKey(publicKeyBytes)
	if err != nil {
		return nil, err
	}

	return publicKey, nil
}

func DecodeKey(serializedKey string) ([]byte, error) {
	bytes, err := hex.DecodeString(TrimPrivateKey(TrimPublicKey(serializedKey)))
	if err != nil {
		_, bytesToBits, err := bech32.Decode(serializedKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode key from hex or bech32: %v", err)
		}

		bytes, err = bech32.ConvertBits(bytesToBits, 5, 8, false)
		if err != nil {
			return nil, fmt.Errorf("failed to decode key from hex or bech32: %v", err)
		}
	}

	return bytes, nil
}

const PublicKeyPrefix = "npub1"
const PrivateKeyPrefix = "nsec1"

func TrimPrivateKey(privateKey string) string {
	return strings.TrimPrefix(privateKey, PrivateKeyPrefix)
}

func TrimPublicKey(publicKey string) string {
	return strings.TrimPrefix(publicKey, PublicKeyPrefix)
}