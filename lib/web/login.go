package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
)

var jwtKey = []byte("zambia_nostr_token")

func generateChallenge() (string, string, error) {
	timestamp := time.Now().Format(time.RFC3339Nano)
	letters := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	challenge := make([]byte, 12)
	_, err := rand.Read(challenge)
	if err != nil {
		return "", "", err
	}
	for i := range challenge {
		challenge[i] = letters[challenge[i]%byte(len(letters))]
	}
	fullChallenge := fmt.Sprintf("%s-%s", string(challenge), timestamp)
	hash := sha256.Sum256([]byte(fullChallenge))
	return fullChallenge, hex.EncodeToString(hash[:]), nil
}

func handleLogin(c *fiber.Ctx) error {
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
	if err := db.Where("email = ?", loginPayload.Email).First(&user).Error; err != nil {
		log.Printf("User not found: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid email or password",
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

	return c.JSON(fiber.Map{
		"event": event,
	})
}

// package web

// import (
// 	"crypto/rand"
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"fmt"
// 	"log"
// 	"time"

// 	"golang.org/x/crypto/bcrypt"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/nbd-wtf/go-nostr"
// )

// var jwtKey = []byte("zambia_nostr_token")

// func generateChallenge() (string, string, error) {
// 	timestamp := time.Now().Format(time.RFC3339Nano)
// 	letters := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
// 	challenge := make([]byte, 12)
// 	_, err := rand.Read(challenge)
// 	if err != nil {
// 		return "", "", err
// 	}
// 	for i := range challenge {
// 		challenge[i] = letters[challenge[i]%byte(len(letters))]
// 	}
// 	fullChallenge := fmt.Sprintf("%s-%s", string(challenge), timestamp)
// 	hash := sha256.Sum256([]byte(fullChallenge))
// 	return fullChallenge, hex.EncodeToString(hash[:]), nil
// }

// func handleLogin(c *fiber.Ctx) error {
// 	log.Println("Login request received")
// 	var loginPayload struct {
// 		types.LoginPayload
// 		Npub string `json:"npub"`
// 	}

// 	if err := c.BodyParser(&loginPayload); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	db, err := graviton.InitGorm()
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	var user types.User
// 	if err := db.Where("email = ?", loginPayload.Email).First(&user).Error; err != nil {
// 		log.Printf("User not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
// 		})
// 	}

// 	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginPayload.Password)); err != nil {
// 		log.Printf("Invalid password: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
// 		})
// 	}

// 	challenge, hash, err := generateChallenge()
// 	if err != nil {
// 		log.Printf("Error generating challenge: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	// Generate Nostr event
// 	event := &nostr.Event{
// 		PubKey:    user.Npub,
// 		CreatedAt: nostr.Timestamp(time.Now().Unix()),
// 		Kind:      1, // Example kind
// 		Tags:      nostr.Tags{},
// 		Content:   challenge,
// 	}

// 	userChallenge := types.UserChallenge{
// 		UserID:    user.ID,
// 		Npub:      user.Npub,
// 		Challenge: challenge,
// 		Hash:      hash,
// 	}
// 	if err := db.Create(&userChallenge).Error; err != nil {
// 		log.Printf("Failed to save challenge: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	return c.JSON(fiber.Map{
// 		"event": event,
// 	})
// }

// package web

// import (
// 	"crypto/rand"
// 	"crypto/sha256"
// 	"encoding/hex"
// 	"fmt"
// 	"log"
// 	"time"

// 	"golang.org/x/crypto/bcrypt"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
// 	"github.com/gofiber/fiber/v2"
// )

// var jwtKey = []byte("zambia_nostr_token")

// func generateChallenge() (string, string, error) {
// 	timestamp := time.Now().Format(time.RFC3339Nano)
// 	letters := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
// 	challenge := make([]byte, 12)
// 	_, err := rand.Read(challenge)
// 	if err != nil {
// 		return "", "", err
// 	}
// 	for i := range challenge {
// 		challenge[i] = letters[challenge[i]%byte(len(letters))]
// 	}
// 	fullChallenge := fmt.Sprintf("%s-%s", string(challenge), timestamp)
// 	hash := sha256.Sum256([]byte(fullChallenge))
// 	return fullChallenge, hex.EncodeToString(hash[:]), nil
// }

// func handleLogin(c *fiber.Ctx) error {
// 	log.Println("Login request received")
// 	var loginPayload struct {
// 		types.LoginPayload
// 		Npub string `json:"npub"`
// 	}

// 	if err := c.BodyParser(&loginPayload); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	db, err := graviton.InitGorm()
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	var user types.User
// 	if err := db.Where("email = ?", loginPayload.Email).First(&user).Error; err != nil {
// 		log.Printf("User not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
// 		})
// 	}

// 	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginPayload.Password)); err != nil {
// 		log.Printf("Invalid password: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
// 		})
// 	}

// 	challenge, hash, err := generateChallenge()
// 	if err != nil {
// 		log.Printf("Error generating challenge: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	userChallenge := types.UserChallenge{
// 		UserID:    user.ID,
// 		Npub:      user.Npub,
// 		Challenge: challenge,
// 		Hash:      hash,
// 	}
// 	if err := db.Create(&userChallenge).Error; err != nil {
// 		log.Printf("Failed to save challenge: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	return c.JSON(fiber.Map{
// 		"challenge": challenge,
// 	})
// }

// package web

// import (
// 	"encoding/hex"
// 	"log"
// 	"time"

// 	"github.com/btcsuite/btcd/btcec/v2/schnorr"
// 	"github.com/golang-jwt/jwt/v4"
// 	"golang.org/x/crypto/bcrypt"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
// 	"github.com/btcsuite/btcd/btcec/v2"
// 	"github.com/btcsuite/btcd/btcutil/bech32"
// 	"github.com/decred/dcrd/dcrec/secp256k1/v4"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/spf13/viper"
// )

// var jwtKey = []byte("zambia_nostr_token")

// func handleLogin(c *fiber.Ctx) error {
// 	log.Println("Login request received")
// 	var loginPayload struct {
// 		types.LoginPayload
// 		SignedMessage struct {
// 			MessageHash string `json:"messageHash"`
// 			Signature   string `json:"signature"`
// 		} `json:"signedMessage"`
// 	}

// 	if err := c.BodyParser(&loginPayload); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	log.Printf("Received signed message: %v", loginPayload.SignedMessage)

// 	// Decode the hex string back to bytes
// 	signatureBytes, err := hex.DecodeString(loginPayload.SignedMessage.Signature)
// 	if err != nil {
// 		log.Println("Error decoding signature hex string:", err)
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Invalid signature format",
// 		})
// 	}

// 	// Parse the signature
// 	signature, err := schnorr.ParseSignature(signatureBytes)
// 	if err != nil {
// 		log.Println("Problems parsing signature", err)
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Malformed signature",
// 		})
// 	}

// 	priv := viper.GetString("key")
// 	privKey := deserializePrivateKey(priv)
// 	stringPrivKey := hex.EncodeToString(privKey.Serialize())
// 	log.Println("Print Priv Key", stringPrivKey)

// 	// Verify the signature
// 	messageHashBytes, err := hex.DecodeString(loginPayload.SignedMessage.MessageHash)
// 	if err != nil {
// 		log.Println("Error decoding message hash hex string:", err)
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Invalid message hash format",
// 		})
// 	}

// 	verify := signature.Verify(messageHashBytes, privKey.PubKey())
// 	if verify {
// 		log.Println("The signature is verified.")
// 	} else {
// 		log.Println("The signature failed verification.")
// 	}

// 	db, err := graviton.InitGorm()
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	var user User
// 	if err := db.Where("email = ?", loginPayload.Email).First(&user).Error; err != nil {
// 		log.Printf("User not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
// 		})
// 	}

// 	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginPayload.Password)); err != nil {
// 		log.Printf("Invalid password: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
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

// 	return c.JSON(fiber.Map{
// 		"token": tokenString,
// 		"user":  user,
// 	})
// }

// func deserializePrivateKey(encodedKey string) *secp256k1.PrivateKey {
// 	_, bytesToBits, err := bech32.Decode(encodedKey)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	privateKeyBytes, err := bech32.ConvertBits(bytesToBits, 5, 8, false)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	privateKey, _ := btcec.PrivKeyFromBytes(privateKeyBytes)
// 	return privateKey
// }

// package web

// import (
// 	"encoding/hex"
// 	"log"
// 	"time"

// 	"github.com/btcsuite/btcd/btcec/v2/schnorr"
// 	"github.com/golang-jwt/jwt/v4"
// 	"golang.org/x/crypto/bcrypt"

// 	types "github.com/HORNET-Storage/hornet-storage/lib"
// 	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
// 	"github.com/btcsuite/btcd/btcec/v2"
// 	"github.com/btcsuite/btcd/btcutil/bech32"
// 	"github.com/decred/dcrd/dcrec/secp256k1/v4"
// 	"github.com/gofiber/fiber/v2"
// 	"github.com/spf13/viper"
// )

// var jwtKey = []byte("zambia_nostr_token")

// func handleLogin(c *fiber.Ctx) error {
// 	log.Println("Login request received")
// 	var loginPayload struct {
// 		types.LoginPayload
// 		SignedMessage struct {
// 			MessageHash string `json:"messageHash"`
// 			Signature   string `json:"signature"`
// 		} `json:"signedMessage"`
// 	}

// 	if err := c.BodyParser(&loginPayload); err != nil {
// 		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
// 			"error": "Cannot parse JSON",
// 		})
// 	}

// 	log.Printf("Received signed message: %v", loginPayload.SignedMessage)

// 	signatureBytes := []byte(loginPayload.SignedMessage.Signature)
// 	signature, err := schnorr.ParseSignature(signatureBytes)
// 	if err != nil {
// 		log.Println("Problems parsing signature", err)
// 	}

// 	priv := viper.GetString("key")
// 	privKey := deserializePrivateKey(priv)
// 	stringPrivKey := hex.EncodeToString(privKey.Serialize())
// 	log.Println("Print Priv Key", stringPrivKey)

// 	verify := signature.Verify([]byte(loginPayload.SignedMessage.MessageHash), privKey.PubKey())

// 	if verify {
// 		log.Println("The signature is verified.")
// 	} else {
// 		log.Println("The signature failed verification.")
// 	}

// 	db, err := graviton.InitGorm()
// 	if err != nil {
// 		log.Printf("Failed to connect to the database: %v", err)
// 		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
// 	}

// 	var user User
// 	if err := db.Where("email = ?", loginPayload.Email).First(&user).Error; err != nil {
// 		log.Printf("User not found: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
// 		})
// 	}

// 	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(loginPayload.Password)); err != nil {
// 		log.Printf("Invalid password: %v", err)
// 		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
// 			"error": "Invalid email or password",
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

// 	return c.JSON(fiber.Map{
// 		"token": tokenString,
// 		"user":  user,
// 	})
// }

// func deserializePrivateKey(encodedKey string) *secp256k1.PrivateKey {
// 	_, bytesToBits, err := bech32.Decode(encodedKey)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	privateKeyBytes, err := bech32.ConvertBits(bytesToBits, 5, 8, false)
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	privateKey, _ := btcec.PrivKeyFromBytes(privateKeyBytes)
// 	return privateKey
// }
