package web

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func StartServer() error {
	app := fiber.New()

	app.Use(cors.New())
	app.Static("/", "./web/panel/build")

	return app.Listen(":5000")
}
