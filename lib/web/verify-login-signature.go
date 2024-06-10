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

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/gofiber/fiber/v2"
)

func handleVerify(c *fiber.Ctx) error {
	log.Println("Verify request received")
	var verifyPayload struct {
		Challenge   string      `json:"challenge"`
		Signature   string      `json:"signature"`
		MessageHash string      `json:"messageHash"`
		Event       nostr.Event `json:"event"`
	}

	if err := c.BodyParser(&verifyPayload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	event := verifyPayload.Event
	log.Println("Event received:", event)

	// Hash the event content
	eventContentHash := sha256.Sum256([]byte(event.Content))
	hashString := hex.EncodeToString(eventContentHash[:])

	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	var userChallenge types.UserChallenge
	if err := db.Where("hash = ? AND expired = ?", hashString, false).First(&userChallenge).Error; err != nil {
		log.Printf("Challenge not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid challenge or challenge expired",
		})
	}

	// Check if the challenge is expired
	if time.Since(userChallenge.CreatedAt) > 3*time.Minute {
		db.Model(&userChallenge).Update("expired", true)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Challenge expired",
		})
	}

	// Verify event
	if !verifyEvent(&event) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid event signature",
		})
	}

	var user types.User
	if err := db.Where("id = ?", userChallenge.UserID).First(&user).Error; err != nil {
		log.Printf("User not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &types.JWTClaims{
		UserID: user.ID,
		Email:  user.Email,
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

	log.Println("Username", user.FirstName)

	return c.JSON(fiber.Map{
		"token": tokenString,
		"user":  user,
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

// package web

// import (
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"encoding/json"
// 	"fmt"
// 	"log"
// 	"strings"
// 	"time"

// 	"github.com/btcsuite/btcd/btcec/v2/schnorr"
// 	"github.com/golang-jwt/jwt/v4"
// 	"github.com/nbd-wtf/go-nostr"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
// 	"github.com/btcsuite/btcd/btcutil/bech32"
// 	"github.com/decred/dcrd/dcrec/secp256k1/v4"
// 	"github.com/gofiber/fiber/v2"
// )

// func handleVerify(c *fiber.Ctx) error {
// 	log.Println("Verify request received")
// 	var verifyPayload struct {
// 		Event nostr.Event `json:"event"`
// 	}

// 	if err := c.BodyParser(&verifyPayload); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	event := verifyPayload.Event
// 	log.Println("Event received:", event)

// 	db, err := graviton.InitGorm()
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	var userChallenge types.UserChallenge
// 	if err := db.Where("hash = ? AND expired = ?", event.ID, false).First(&userChallenge).Error; err != nil {
// 		log.Printf("Challenge not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid challenge or challenge expired",
// 		})
// 	}

// 	// Check if the challenge is expired
// 	if time.Since(userChallenge.CreatedAt) > 3*time.Minute {
// 		db.Model(&userChallenge).Update("expired", true)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Challenge expired",
// 		})
// 	}

// 	// Verify event
// 	if !verifyEvent(&event) {
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid event signature",
// 		})
// 	}

// 	var user types.User
// 	if err := db.Where("id = ?", userChallenge.UserID).First(&user).Error; err != nil {
// 		log.Printf("User not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "User not found",
// 		})
// 	}

// 	expirationTime := time.Now().Add(24 * time.Hour)
// 	claims := &types.JWTClaims{
// 		UserID: user.ID,
// 		Email:  user.Email,
// 		RegisteredClaims: jwt.RegisteredClaims{
// 			ExpiresAt: jwt.NewNumericDate(expirationTime),
// 		},
// 	}

// 	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
// 	tokenString, err := token.SignedString(jwtKey)
// 	if err != nil {
// 		log.Printf("Error creating JWT token: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"error": "Error creating token",
// 		})
// 	}

// 	log.Println("Username", user.FirstName)

// 	return c.JSON(fiber.Map{
// 		"token": tokenString,
// 		"user":  user,
// 	})
// }

// func verifyEvent(event *nostr.Event) bool {
// 	serialized := serializeEventForID(event)
// 	log.Println("The Event ID is:", event.ID)
// 	match, hash := HashAndCompare(serialized, event.ID)
// 	if match {
// 		fmt.Println("Hash matches ID:", event.ID)
// 	} else {
// 		fmt.Println("Hash does not match ID")
// 	}
// 	signatureBytes, _ := hex.DecodeString(event.Sig)
// 	cleanSignature, _ := schnorr.ParseSignature(signatureBytes)
// 	publicSignatureBytes, _ := hex.DecodeString(event.PubKey)

// 	cleanPublicKey, _ := schnorr.ParsePubKey(publicSignatureBytes)

// 	isValid := cleanSignature.Verify(hash[:], cleanPublicKey)

// 	if isValid {
// 		fmt.Println("Signature is valid from my implementation")
// 	} else {
// 		fmt.Println("Signature is invalid from my implementation")
// 	}

// 	log.Println("Event tags: ", event.Tags)

// 	isValid, err := event.CheckSignature()
// 	if err != nil {
// 		log.Println("Error checking signature:", err)
// 		return false
// 	}
// 	if isValid {
// 		fmt.Println("Signature is valid")
// 	} else {
// 		fmt.Println("Signature is invalid")
// 	}

// 	if isValid && match {
// 		return true
// 	} else {
// 		return false
// 	}
// }

// func serializeEventForID(event *nostr.Event) []byte {
// 	// Assuming the Tags and other fields are already correctly filled except ID and Sig
// 	serialized, err := json.Marshal([]interface{}{
// 		0,
// 		event.PubKey,
// 		event.CreatedAt,
// 		event.Kind,
// 		event.Tags,
// 		event.Content,
// 	})
// 	if err != nil {
// 		panic(err) // Handle error properly in real code
// 	}

// 	return serialized
// }

// func HashAndCompare(data []byte, hash string) (bool, []byte) {
// 	h := sha256.Sum256(data)
// 	return hex.EncodeToString(h[:]) == hash, h[:]
// }

// func DeserializePublicKey(serializedKey string) (*secp256k1.PublicKey, error) {
// 	publicKeyBytes, err := DecodeKey(serializedKey)
// 	if err != nil {
// 		return nil, err
// 	}

// 	publicKey, err := schnorr.ParsePubKey(publicKeyBytes)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return publicKey, nil
// }

// func DecodeKey(serializedKey string) ([]byte, error) {
// 	bytes, err := hex.DecodeString(TrimPrivateKey(TrimPublicKey(serializedKey)))
// 	if err != nil {
// 		_, bytesToBits, err := bech32.Decode(serializedKey)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to decode key from hex or bech32: %v", err)
// 		}

// 		bytes, err = bech32.ConvertBits(bytesToBits, 5, 8, false)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to decode key from hex or bech32: %v", err)
// 		}
// 	}

// 	return bytes, nil
// }

// const PublicKeyPrefix = "npub1"
// const PrivateKeyPrefix = "nsec1"

// func TrimPrivateKey(privateKey string) string {
// 	return strings.TrimPrefix(privateKey, PrivateKeyPrefix)
// }

// func TrimPublicKey(publicKey string) string {
// 	return strings.TrimPrefix(publicKey, PublicKeyPrefix)
// }

// package web

// import (
// 	"encoding/hex"
// 	"fmt"
// 	"log"
// 	"strings"
// 	"time"

// 	"github.com/btcsuite/btcd/btcec/v2/schnorr"
// 	"github.com/golang-jwt/jwt/v4"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
// 	"github.com/btcsuite/btcd/btcutil/bech32"
// 	"github.com/decred/dcrd/dcrec/secp256k1/v4"
// 	"github.com/gofiber/fiber/v2"
// )

// func handleVerify(c *fiber.Ctx) error {
// 	log.Println("Verify request received")
// var verifyPayload struct {
// 	Challenge   string `json:"challenge"`
// 	Signature   string `json:"signature"`
// 	MessageHash string `json:"messageHash"`
// }

// 	if err := c.BodyParser(&verifyPayload); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	log.Println("Challenge", verifyPayload.Challenge)
// 	log.Println("Signature", verifyPayload.Signature)
// 	log.Println("MessageHash", verifyPayload.MessageHash)

// 	db, err := graviton.InitGorm()
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	var userChallenge types.UserChallenge
// 	if err := db.Where("hash = ? AND expired = ?", verifyPayload.MessageHash, false).First(&userChallenge).Error; err != nil {
// 		log.Printf("Challenge not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid challenge or challenge expired",
// 		})
// 	}

// 	// Check if the challenge is expired
// 	if time.Since(userChallenge.CreatedAt) > 3*time.Minute {
// 		db.Model(&userChallenge).Update("expired", true)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Challenge expired",
// 		})
// 	}

// 	signatureBytes, err := hex.DecodeString(verifyPayload.Signature)
// 	if err != nil {
// 		log.Println("Error decoding signature hex string:", err)
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Invalid signature format",
// 		})
// 	}

// 	signature, err := schnorr.ParseSignature(signatureBytes)
// 	if err != nil {
// 		log.Println("Problems parsing signature", err)
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Malformed signature",
// 		})
// 	}

// 	messageHashBytes, err := hex.DecodeString(verifyPayload.MessageHash)
// 	if err != nil {
// 		log.Println("Error decoding message hash hex string:", err)
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Invalid message hash format",
// 		})
// 	}

// 	pubKey, err := DeserializePublicKey(userChallenge.Npub)
// 	if err != nil {
// 		log.Println(err)
// 	}
// 	verify := signature.Verify(messageHashBytes, pubKey)
// 	if verify {
// 		log.Println("The signature is verified.")
// 	} else {
// 		log.Println("The signature failed verification.")
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid signature",
// 		})
// 	}

// 	var user types.User
// 	if err := db.Where("id = ?", userChallenge.UserID).First(&user).Error; err != nil {
// 		log.Printf("User not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "User not found",
// 		})
// 	}

// 	expirationTime := time.Now().Add(24 * time.Hour)
// 	claims := &types.JWTClaims{
// 		UserID: user.ID,
// 		Email:  user.Email,
// 		RegisteredClaims: jwt.RegisteredClaims{
// 			ExpiresAt: jwt.NewNumericDate(expirationTime),
// 		},
// 	}

// 	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
// 	tokenString, err := token.SignedString(jwtKey)
// 	if err != nil {
// 		log.Printf("Error creating JWT token: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
// 			"error": "Error creating token",
// 		})
// 	}

// 	log.Println("Username", user.FirstName)

// 	return c.JSON(fiber.Map{
// 		"token": tokenString,
// 		"user":  user,
// 	})
// }

// func DeserializePublicKey(serializedKey string) (*secp256k1.PublicKey, error) {
// 	publicKeyBytes, err := DecodeKey(serializedKey)
// 	if err != nil {
// 		return nil, err
// 	}

// 	publicKey, err := schnorr.ParsePubKey(publicKeyBytes)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return publicKey, nil
// }

// func DecodeKey(serializedKey string) ([]byte, error) {
// 	bytes, err := hex.DecodeString(TrimPrivateKey(TrimPublicKey(serializedKey)))
// 	if err != nil {
// 		_, bytesToBits, err := bech32.Decode(serializedKey)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to decode key from hex or bech32: %v", err)
// 		}

// 		bytes, err = bech32.ConvertBits(bytesToBits, 5, 8, false)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to decode key from hex or bech32: %v", err)
// 		}
// 	}

// 	return bytes, nil
// }

// const PublicKeyPrefix = "npub1"
// const PrivateKeyPrefix = "nsec1"

// func TrimPrivateKey(privateKey string) string {
// 	return strings.TrimPrefix(privateKey, PrivateKeyPrefix)
// }

// func TrimPublicKey(publicKey string) string {
// 	return strings.TrimPrefix(publicKey, PublicKeyPrefix)
// }
