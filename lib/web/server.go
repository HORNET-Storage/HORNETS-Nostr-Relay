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

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func StartServer(store stores.Store) error {
	app := fiber.New()

	go pullBitcoinPrice()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // You can restrict this to specific origins if needed, e.g., "http://localhost:3000"
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, OPTIONS",
	}))

	// Rate limited routes
	app.Post("/signup", func(c *fiber.Ctx) error {
		return signUpUser(c, store)
	})
	app.Post("/login", func(c *fiber.Ctx) error {
		return loginUser(c, store)
	})

	app.Post("/verify", rateLimiterMiddleware(), func(c *fiber.Ctx) error {
		return verifyLoginSignature(c, store)
	})

	// Open routes
	app.Get("/user-exist", func(c *fiber.Ctx) error {
		return checkUserExists(c, store)
	})
	app.Post("/logout", func(c *fiber.Ctx) error {
		return logoutUser(c, store)
	})

	// Wallet-specific routes with API key authentication
	walletRoutes := app.Group("/api/wallet")
	walletRoutes.Use(apiKeyMiddleware)
	walletRoutes.Post("/balance", func(c *fiber.Ctx) error {
		return updateWalletBalance(c, store)
	})
	walletRoutes.Post("/transactions", func(c *fiber.Ctx) error {
		return updateWalletTransactions(c, store)
	})
	walletRoutes.Post("/addresses", func(c *fiber.Ctx) error {
		return saveWalletAddresses(c, store) // Pass the store instance
	})

	secured := app.Group("/api")
	secured.Use(func(c *fiber.Ctx) error {
		return jwtMiddleware(c, store)
	})

	// Dedicated routes for each handler
	secured.Get("/relaycount", func(c *fiber.Ctx) error {
		return getRelayCount(c, store)
	})
	secured.Post("/relay-settings", updateRelaySettings)
	secured.Get("/relay-settings", getRelaySettings)
	secured.Get("/timeseries", func(c *fiber.Ctx) error {
		return getProfilesTimeSeriesData(c, store)
	})
	secured.Get("/activitydata", func(c *fiber.Ctx) error {
		return getMonthlyStorageStats(c, store)
	})
	secured.Get("/barchartdata", func(c *fiber.Ctx) error {
		return getNotesMediaStorageData(c, store)
	})
	secured.Post("/updateRate", func(c *fiber.Ctx) error {
		return updateBitcoinRate(c, store)
	})
	secured.Get("/balance/usd", func(c *fiber.Ctx) error {
		return getWalletBalanceUSD(c, store)
	})

	secured.Get("/transactions/latest", func(c *fiber.Ctx) error {
		return getLatestWalletTransactions(c, store)
	})
	secured.Get("/bitcoin-rates/last-30-days", func(c *fiber.Ctx) error {
		return getBitcoinRatesLast30Days(c, store)
	})
	secured.Get("/addresses", func(c *fiber.Ctx) error {
		return pullWalletAddresses(c, store)
	})
	secured.Get("/kinds", func(c *fiber.Ctx) error {
		return getKindData(c, store)
	})
	secured.Get("/kind-trend/:kindNumber", func(c *fiber.Ctx) error {
		return getKindTrendData(c, store)
	})
	secured.Post("/pending-transactions", func(c *fiber.Ctx) error {
		return saveUnconfirmedTransaction(c, store)
	})
	secured.Post("/replacement-transactions", func(c *fiber.Ctx) error {
		return replaceTransaction(c, store)
	})
	secured.Get("/pending-transactions", func(c *fiber.Ctx) error {
		return getPendingTransactions(c, store)
	})
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
