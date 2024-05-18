package web

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func StartServer() error {
	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Public routes
	app.Post("/login", handleLogin)
	app.Post("/signup", handleSignUp)

	// Protected routes
	app.Use(jwtMiddleware)

	// Dedicated routes for each handler
	app.Post("/relaycount", handleRelayCount)
	app.Post("/relay-settings", handleRelaySettings)
	app.Post("/timeseries", handleTimeSeries)
	app.Post("/activitydata", handleActivityData)
	app.Post("/barchartdata", handleBarChartData)

	return app.Listen(":5000")
}
