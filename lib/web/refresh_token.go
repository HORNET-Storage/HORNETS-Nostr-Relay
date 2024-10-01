package web

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

func refreshToken(c *fiber.Ctx) error {
	user := c.Locals("user").(*types.JWTClaims)

	// Create a new token
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &types.JWTClaims{
		UserID: user.UserID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Could not generate token",
		})
	}

	return c.JSON(fiber.Map{
		"token": tokenString,
	})
}
