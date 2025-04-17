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

	go pullBitcoinPrice(store)

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // You can restrict this to specific origins if needed, e.g., "http://localhost:3000"
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, DELETE, PUT, OPTIONS",
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

	// Only apply API key middleware if not in demo mode
	if !viper.GetBool("demo_mode") {
		walletRoutes.Use(apiKeyMiddleware)
		log.Println("API key authentication enabled for wallet routes")
	} else {
		log.Println("WARNING: Running in demo mode - wallet API routes are UNSECURED!")
	}
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

	// Only apply JWT middleware if not in demo mode
	if !viper.GetBool("demo_mode") {
		secured.Use(func(c *fiber.Ctx) error {
			return jwtMiddleware(c, store)
		})
		log.Println("JWT authentication enabled for API routes")
	} else {
		log.Println("WARNING: Running in demo mode - API routes are UNSECURED!")
	}

	// Dedicated routes for each handler
	secured.Get("/relaycount", func(c *fiber.Ctx) error {
		return getRelayCount(c, store)
	})
	secured.Post("/relay-settings", func(c *fiber.Ctx) error {
		return updateRelaySettings(c, store)
	})

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
	secured.Get("/paid-subscriber-profiles", func(c *fiber.Ctx) error {
		return HandleGetPaidSubscriberProfiles(c, store)
	})
	secured.Post("/refresh-token", refreshToken)

	secured.Get("/files", func(c *fiber.Ctx) error {
		return HandleGetFilesByType(c, store)
	})

	// Moderation notification routes
	secured.Get("/moderation/notifications", func(c *fiber.Ctx) error {
		return getModerationNotifications(c, store)
	})
	secured.Post("/moderation/notifications/read", func(c *fiber.Ctx) error {
		return markNotificationAsRead(c, store)
	})
	secured.Post("/moderation/notifications/read-all", func(c *fiber.Ctx) error {
		return markAllNotificationsAsRead(c, store)
	})
	secured.Get("/moderation/stats", func(c *fiber.Ctx) error {
		return getModerationStats(c, store)
	})
	secured.Post("/moderation/notifications", func(c *fiber.Ctx) error {
		return createModerationNotification(c, store)
	})
	secured.Get("/moderation/blocked-event/:id", func(c *fiber.Ctx) error {
		return getBlockedEvent(c, store)
	})
	secured.Post("/moderation/unblock", func(c *fiber.Ctx) error {
		return unblockEvent(c, store)
	})
	secured.Delete("/moderation/event/:id", func(c *fiber.Ctx) error {
		return deleteModeratedEvent(c, store)
	})

	// Payment notification routes
	secured.Get("/payment/notifications", func(c *fiber.Ctx) error {
		return getPaymentNotifications(c, store)
	})
	secured.Post("/payment/notifications/read", func(c *fiber.Ctx) error {
		return markPaymentNotificationAsRead(c, store)
	})
	secured.Post("/payment/notifications/read-all", func(c *fiber.Ctx) error {
		return markAllPaymentNotificationsAsRead(c, store)
	})
	secured.Get("/payment/stats", func(c *fiber.Ctx) error {
		return getPaymentStats(c, store)
	})
	secured.Post("/payment/notifications", func(c *fiber.Ctx) error {
		return createPaymentNotification(c, store)
	})

	// Report notification routes
	secured.Get("/reports/notifications", func(c *fiber.Ctx) error {
		return getReportNotifications(c, store)
	})
	secured.Post("/reports/notifications/read", func(c *fiber.Ctx) error {
		return markReportNotificationAsRead(c, store)
	})
	secured.Post("/reports/notifications/read-all", func(c *fiber.Ctx) error {
		return markAllReportNotificationsAsRead(c, store)
	})
	secured.Get("/reports/stats", func(c *fiber.Ctx) error {
		return getReportStats(c, store)
	})
	secured.Get("/reports/event/:id", func(c *fiber.Ctx) error {
		return getReportedEvent(c, store)
	})
	secured.Delete("/reports/event/:id", func(c *fiber.Ctx) error {
		return deleteReportedEvent(c, store)
	})

	// Blocked pubkeys routes
	secured.Get("/blocked-pubkeys", func(c *fiber.Ctx) error {
		return getBlockedPubkeys(c, store)
	})
	secured.Post("/blocked-pubkeys", func(c *fiber.Ctx) error {
		return blockPubkey(c, store)
	})
	secured.Delete("/blocked-pubkeys/:pubkey", func(c *fiber.Ctx) error {
		return unblockPubkey(c, store)
	})

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
