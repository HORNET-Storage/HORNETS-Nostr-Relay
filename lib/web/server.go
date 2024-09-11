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
		AllowOrigins: "*", // You can restrict this to specific origins if needed, e.g., "http://localhost:3000"
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, OPTIONS",
	}))

	// Rate limited routes
	app.Post("/signup", rateLimiterMiddleware(), signUpUser)
	app.Post("/login", rateLimiterMiddleware(), loginUser)
	app.Post("/verify", rateLimiterMiddleware(), verifyLoginSignature)

	// Open routes
	app.Get("/user-exist", checkUserExists)
	app.Post("/logout", logoutUser)

	secured := app.Group("/api")
	secured.Use(jwtMiddleware)

	// Dedicated routes for each handler
	secured.Get("/relaycount", getRelayCount)
	secured.Post("/relay-settings", updateRelaySettings)
	secured.Get("/relay-settings", getRelaySettings)
	secured.Get("/timeseries", getProfilesTimeSeriesData)
	secured.Get("/activitydata", getMonthlyStorageStats)
	secured.Get("/barchartdata", getNotesMediaStorageData)
	secured.Post("/balance", updateWalletBalance)           // TODO: We need to handle this one slightly differently
	secured.Post("/transactions", updateWalletTransactions) // TODO: We need to handle this one slightly differently
	secured.Post("/updateRate", updateBitcoinRate)          // TODO: We need to handle this one slightly differently
	secured.Get("/balance/usd", getWalletBalanceUSD)
	secured.Get("/transactions/latest", getLatestWalletTransactions)
	secured.Get("/bitcoin-rates/last-30-days", getBitcoinRatesLast30Days)
	secured.Post("/addresses", saveWalletAddresses) // TODO: We need to handle this one slightly differently
	secured.Get("/addresses", pullWalletAddresses)
	secured.Get("/kinds", getKindData)
	secured.Get("/kind-trend/:kindNumber", getKindTrendData)
	secured.Post("/pending-transactions", saveUnconfirmedTransaction)
	secured.Post("/replacement-transactions", replaceTransaction)
	secured.Get("/pending-transactions", getPendingTransactions)
	secured.Post("/refresh-token", refreshToken)

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
