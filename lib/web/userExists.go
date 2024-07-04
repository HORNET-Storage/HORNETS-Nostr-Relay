package web

import (
	"log"
	"net/http"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/gofiber/fiber/v2"
)

func userExist(c *fiber.Ctx) error {
	log.Println("Checking if user exists...")
	db, err := graviton.InitGorm()
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Internal Server Error")
	}

	var user types.User
	if err := db.First(&user).Error; err != nil {
		return c.JSON(fiber.Map{"exists": false})
	}

	return c.JSON(fiber.Map{"exists": true})
}
