package web

import (
	"fmt"
	"log"
	"net/http"
	"strconv"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/spf13/viper"
	"gorm.io/gorm"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
)

func StartServer(store stores.Store, statsDb *gorm.DB) error {
	app := fiber.New()

	go pullBitcoinPrice()

	statisticsStore := &gorm.GormStatisticsStore{}

	err := statisticsStore.InitStore(viper.GetString("relay_stats_db"), nil)
	if err != nil {
		log.Fatalf("failed to initialize statistics store: %v", err)
	}

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // You can restrict this to specific origins if needed, e.g., "http://localhost:3000"
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, OPTIONS",
	}))

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("db", statsDb)
		c.Locals("store", store)
		return c.Next()
	})

	// Rate limited routes
	app.Post("/signup", func(c *fiber.Ctx) error {
		return signUpUser(c, statisticsStore)
	})
	app.Post("/login", func(c *fiber.Ctx) error {
		return loginUser(c, statisticsStore)
	})

	app.Post("/verify", rateLimiterMiddleware(), func(c *fiber.Ctx) error {
		return verifyLoginSignature(c, statisticsStore)
	})

	// Open routes
	app.Get("/user-exist", func(c *fiber.Ctx) error {
		return checkUserExists(c, statisticsStore)
	})
	app.Post("/logout", func(c *fiber.Ctx) error {
		return logoutUser(c, statisticsStore)
	})

	// Wallet-specific routes with API key authentication
	walletRoutes := app.Group("/api/wallet")
	walletRoutes.Use(apiKeyMiddleware)
	walletRoutes.Post("/balance", func(c *fiber.Ctx) error {
		return updateWalletBalance(c, statisticsStore)
	})
	walletRoutes.Post("/transactions", func(c *fiber.Ctx) error {
		return updateWalletTransactions(c, statisticsStore)
	})
	walletRoutes.Post("/addresses", func(c *fiber.Ctx) error {
		return saveWalletAddresses(c, statisticsStore) // Pass the store instance
	})

	secured := app.Group("/api")
	secured.Use(func(c *fiber.Ctx) error {
		return jwtMiddleware(c, statisticsStore)
	})

	// Dedicated routes for each handler
	secured.Get("/relaycount", func(c *fiber.Ctx) error {
		return getRelayCount(c, statisticsStore)
	})
	secured.Post("/relay-settings", updateRelaySettings)
	secured.Get("/relay-settings", getRelaySettings)
	secured.Get("/timeseries", func(c *fiber.Ctx) error {
		return getProfilesTimeSeriesData(c, statisticsStore)
	})
	secured.Get("/activitydata", func(c *fiber.Ctx) error {
		return getMonthlyStorageStats(c, statisticsStore)
	})
	secured.Get("/barchartdata", func(c *fiber.Ctx) error {
		return getNotesMediaStorageData(c, statisticsStore)
	})
	secured.Post("/updateRate", func(c *fiber.Ctx) error {
		return updateBitcoinRate(c, statisticsStore)
	})
	secured.Get("/balance/usd", func(c *fiber.Ctx) error {
		return getWalletBalanceUSD(c, statisticsStore)
	})

	secured.Get("/transactions/latest", func(c *fiber.Ctx) error {
		return getLatestWalletTransactions(c, statisticsStore)
	})
	secured.Get("/bitcoin-rates/last-30-days", func(c *fiber.Ctx) error {
		return getBitcoinRatesLast30Days(c, statisticsStore)
	})
	secured.Get("/addresses", func(c *fiber.Ctx) error {
		return pullWalletAddresses(c, statisticsStore)
	})
	secured.Get("/kinds", func(c *fiber.Ctx) error {
		return getKindData(c, statisticsStore)
	})
	secured.Get("/kind-trend/:kindNumber", func(c *fiber.Ctx) error {
		return getKindTrendData(c, statisticsStore)
	})
	secured.Post("/pending-transactions", func(c *fiber.Ctx) error {
		return saveUnconfirmedTransaction(c, statisticsStore)
	})
	secured.Post("/replacement-transactions", func(c *fiber.Ctx) error {
		return replaceTransaction(c, statisticsStore)
	})
	secured.Get("/pending-transactions", func(c *fiber.Ctx) error {
		return getPendingTransactions(c, statisticsStore)
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
