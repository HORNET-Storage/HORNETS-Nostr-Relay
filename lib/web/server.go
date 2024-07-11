package web

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/spf13/viper"
)

func StartServer() error {
	app := fiber.New()

	go pullBitcoinPrice()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Dedicated routes for each handler
	app.Post("/relaycount", handleRelayCount)
	app.Post("/relay-settings", handleRelaySettings)
	app.Get("/relay-settings", handleGetRelaySettings)
	app.Post("/timeseries", handleTimeSeries)
	app.Post("/activitydata", handleActivityData)
	app.Post("/barchartdata", handleBarChartData)
	app.Post("/balance", handleBalance) // Add the new route here
	app.Post("/transactions", handleTransactions)
	app.Post("/updateRate", handleBitcoinRate)
	app.Get("/balance/usd", handleBalanceInUSD)
	app.Get("/transactions/latest", handleLatestTransactions)
	app.Get("/bitcoin-rates/last-30-days", handleBitcoinRatesForLast30Days)
	app.Post("/addresses", handleAddresses)
	app.Get("/addresses", getAddresses)
	app.Post("/signup", handleSignUp)
	app.Post("/login", handleLogin) // Add the new login route
	app.Post("/verify", handleVerify)
	app.Get("/user-exist", userExist)
	app.Get("/api/kinds", handleKindData)
	app.Get("/api/kind-trend/:kindNumber", handleKindTrendData)

	port := viper.GetString("port")
	p, err := strconv.Atoi(port)
	if err != nil {
		log.Fatal("Error parsing port port")
	}

	app.Use(filesystem.New(filesystem.Config{
		Root:   http.Dir("./web"),
		Browse: false,
		Index:  "index.html",
	}))

	app.Use(func(c *fiber.Ctx) error {
		return c.SendFile("./web/index.html")
	})

	return app.Listen(fmt.Sprintf(":%d", p+2))
}
