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

	// Dedicated routes for each handler
	app.Post("/relaycount", handleRelayCount)
	app.Post("/relay-settings", handleRelaySettings)
	app.Post("/timeseries", handleTimeSeries)
	app.Post("/activitydata", handleActivityData)
	app.Post("/barchartdata", handleBarChartData)
	app.Post("/balance", handleBalance) // Add the new route here
	app.Post("/transactions", handleTransactions)
	app.Post("/updateRate", handleBitcoinRate)
	app.Get("/balance/usd", handleBalanceInUSD)
	app.Get("/transactions/latest", handleLatestTransactions)

	return app.Listen(":5000")
}
