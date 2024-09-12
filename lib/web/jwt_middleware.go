package web

import (
	"fmt"
	"log"
	"strings"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
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
	db, err := graviton.InitGorm()
	if err != nil {
		log.Printf("JWT Middleware: Database connection error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Internal server error",
		})
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
		log.Printf("JWT Middleware: Valid token for user ID: %d", claims.UserID)
		// Add the claims to the context for use in subsequent handlers
		c.Locals("user", claims)
		return c.Next()
	}

	log.Println("JWT Middleware: Invalid token claims")
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": "Invalid token claims",
	})
}
