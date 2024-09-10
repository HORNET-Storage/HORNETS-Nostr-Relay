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
	app.Post("/relaycount", getRelayCount)
	app.Post("/relay-settings", updateRelaySettings)
	app.Get("/relay-settings", getRelaySettings)
	app.Post("/timeseries", getProfilesTimeSeriesData)
	app.Post("/activitydata", getMonthlyStorageStats)
	app.Post("/barchartdata", getNotesMediaStorageData)
	app.Post("/balance", updateWalletBalance) // Add the new route here
	app.Post("/transactions", updateWalletTransactions)
	app.Post("/updateRate", updateBitcoinRate)
	app.Get("/balance/usd", getWalletBalanceUSD)
	app.Get("/transactions/latest", getLatestWalletTransactions)
	app.Get("/bitcoin-rates/last-30-days", getBitcoinRatesLast30Days)
	app.Post("/addresses", saveWalletAddresses)
	app.Get("/addresses", pullWalletAddresses)
	app.Post("/signup", signUpUser)
	app.Post("/login", loginUser) // Add the new login route
	app.Post("/verify", verifyLoginSignature)
	app.Get("/user-exist", checkUserExists)
	app.Get("/api/kinds", getKindData)
	app.Get("/api/kind-trend/:kindNumber", getKindTrendData)
	app.Post("/pending-transactions", saveUnconfirmedTransaction)
	app.Post("/replacement-transactions", replaceTransaction)
	app.Get("/pending-transactions", getPendingTransactions)

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
