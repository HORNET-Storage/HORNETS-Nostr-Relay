package middleware

import (
	"fmt"
	"strings"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/spf13/viper"
)

func JwtMiddleware(c *fiber.Ctx, store stores.Store) error {
	// In demo mode, skip authentication and set a mock user
	if viper.GetBool("demo_mode") {
		// Set a demo user in context
		c.Locals("user", &types.JWTClaims{
			Email:  "demo@example.com",
			UserID: 1,
		})
		return c.Next()
	}

	// Get the Authorization header
	authHeader := c.Get("Authorization")

	// Check if the header is empty or doesn't start with "Bearer "
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		logging.Info("JWT Middleware: Missing or invalid Authorization header")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Missing or invalid Authorization header",
		})
	}

	// Extract the token
	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Check if the token is active and not expired using the store method
	isActive, err := store.GetStatsStore().IsActiveToken(tokenString)
	if err != nil {
		logging.Infof("JWT Middleware: Error checking token: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	if !isActive {
		logging.Info("JWT Middleware: Token not found in ActiveTokens or expired")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &types.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate the algorithm
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(viper.GetString("jwt_secret")), nil
	})

	if err != nil {
		logging.Infof("JWT Middleware: Token parsing error: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid or expired token",
		})
	}

	// Check if the token is valid and set user claims in the context
	if claims, ok := token.Claims.(*types.JWTClaims); ok && token.Valid {
		c.Locals("user", claims)
		return c.Next()
	}

	logging.Info("JWT Middleware: Invalid token claims")
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": "Invalid token claims",
	})
}
