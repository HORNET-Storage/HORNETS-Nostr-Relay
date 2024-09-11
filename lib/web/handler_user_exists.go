package web

import (
	"log"
	"net/http"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
)

func checkUserExists(c *fiber.Ctx) error {
	log.Println("Checking if user exists...")
	db, err := graviton.InitGorm()
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Internal Server Error")
	}

	var user types.User
	if err := db.First(&user).Error; err != nil {
		return c.JSON(fiber.Map{
			"exists":      false,
			"allowSignup": true,
			"message":     "No users found. Signup is allowed.",
		})
	}

	return c.JSON(fiber.Map{
		"exists":      true,
		"allowSignup": false,
		"message":     "User exists. Signup is not allowed.",
	})
}
