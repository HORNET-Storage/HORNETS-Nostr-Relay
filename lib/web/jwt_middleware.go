package web

import (
	"fmt"
	"log"
	"strings"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func jwtMiddleware(c *fiber.Ctx) error {

	// Get the Authorization header
	authHeader := c.Get("Authorization")

	// Check if the header is empty or doesn't start with "Bearer "
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		log.Println("JWT Middleware: Missing or invalid Authorization header")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing or invalid Authorization header",
		})
	}

	// Extract the token
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Check if the token is in ActiveTokens
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

	var activeToken types.ActiveToken
	if err := db.Where("token = ? AND expires_at > ?", tokenString, time.Now()).First(&activeToken).Error; err != nil {
		log.Printf("JWT Middleware: Token not found in ActiveTokens or expired: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &types.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the alg
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtKey, nil
	})

	if err != nil {
		log.Printf("JWT Middleware: Token parsing error: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Check if the token is valid
	if claims, ok := token.Claims.(*types.JWTClaims); ok && token.Valid {
		c.Locals("user", claims)
		return c.Next()
	}

	log.Println("JWT Middleware: Invalid token claims")
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": "Invalid token claims",
	})
}
