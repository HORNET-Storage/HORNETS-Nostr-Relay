package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/handlers/blossom"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"

	// Import the organized handlers
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/access"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/auth"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/bitcoin"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/moderation"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/push"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/settings"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/statistics"
	"github.com/HORNET-Storage/hornet-storage/lib/web/handlers/wallet"

	// Import middleware
	"github.com/HORNET-Storage/hornet-storage/lib/web/middleware"

	// Import services
	"github.com/HORNET-Storage/hornet-storage/lib/web/services"
)

func StartServer(store stores.Store) error {
	logging.Info("Starting web server", map[string]interface{}{
		"port": config.GetPort("web"),
		"demo": config.IsEnabled("demo"),
	})

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			logging.Error("Web server error", map[string]interface{}{
				"error":  err.Error(),
				"path":   c.Path(),
				"method": c.Method(),
			})
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Internal server error",
			})
		},
	})

	// CORS middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, DELETE, PUT, OPTIONS",
	}))

	// Disable compression for static assets to prevent ngrok issues
	app.Use(func(c *fiber.Ctx) error {
		// Set headers to prevent compression for JS/CSS files
		if strings.Contains(c.Path(), ".js") || strings.Contains(c.Path(), ".css") {
			c.Set("Cache-Control", "no-transform")
			c.Set("Content-Encoding", "identity")
			c.Set("Accept-Encoding", "identity")
		}
		return c.Next()
	})

	// Request logging middleware
	app.Use(func(c *fiber.Ctx) error {
		logging.Debug("HTTP Request", map[string]interface{}{
			"method": c.Method(),
			"path":   c.Path(),
			"ip":     c.IP(),
		})
		return c.Next()
	})

	/// ================================
	// BLOSSOM FILE STORAGE ROUTES
	// ================================

	blossomServer := blossom.NewServer(store)
	blossomServer.SetupRoutes(app)

	// ================================
	// AUTHENTICATION ROUTES
	// ================================

	app.Post("/signup", middleware.RateLimiterMiddleware(), func(c *fiber.Ctx) error {
		return auth.SignUpUser(c, store)
	})

	app.Post("/login", middleware.RateLimiterMiddleware(), func(c *fiber.Ctx) error {
		return auth.LoginUser(c, store)
	})

	app.Post("/verify", middleware.RateLimiterMiddleware(), func(c *fiber.Ctx) error {
		return auth.VerifyLoginSignature(c, store)
	})

	app.Get("/user-exist", func(c *fiber.Ctx) error {
		return handlers.CheckUserExists(c, store)
	})

	app.Post("/logout", func(c *fiber.Ctx) error {
		return auth.LogoutUser(c, store)
	})

	// ================================
	// WALLET PROXY ROUTES (MUST BE BEFORE /api/wallet ROUTES)
	// ================================

	// Public wallet authentication routes (no authentication required)
	app.Get("/api/wallet-proxy/challenge", func(c *fiber.Ctx) error {
		return wallet.HandleChallenge(c)
	})

	app.Post("/api/wallet-proxy/verify", func(c *fiber.Ctx) error {
		return wallet.HandleVerify(c)
	})

	// Protected wallet operation routes (JWT required)
	walletProxySecured := app.Group("/api/wallet-proxy")

	if !config.IsEnabled("demo") {
		walletProxySecured.Use(func(c *fiber.Ctx) error {
			return middleware.JwtMiddleware(c, store)
		})
		logging.Info("JWT authentication enabled for protected wallet proxy routes")
	} else {
		logging.Warn("Running in demo mode - protected wallet proxy routes are UNSECURED!")
	}

	walletProxySecured.Get("/panel-health", func(c *fiber.Ctx) error {
		return wallet.HandlePanelHealth(c)
	})

	walletProxySecured.Post("/calculate-tx-size", func(c *fiber.Ctx) error {
		return wallet.HandleCalculateTxSize(c)
	})

	walletProxySecured.Post("/transaction", func(c *fiber.Ctx) error {
		return wallet.HandleTransaction(c)
	})

	// ================================
	// WALLET API ROUTES
	// ================================

	walletRoutes := app.Group("/api/wallet")

	if !config.IsEnabled("demo") {
		walletRoutes.Use(func(c *fiber.Ctx) error {
			return middleware.ApiKeyMiddleware(c)
		})
		logging.Info("API key authentication enabled for wallet routes")
	} else {
		logging.Warn("Running in demo mode - wallet API routes are UNSECURED!")
	}

	walletRoutes.Post("/balance", func(c *fiber.Ctx) error {
		return wallet.UpdateWalletBalance(c, store)
	})

	walletRoutes.Post("/transactions", func(c *fiber.Ctx) error {
		return wallet.UpdateWalletTransactions(c, store)
	})

	walletRoutes.Post("/addresses", func(c *fiber.Ctx) error {
		return wallet.SaveWalletAddresses(c, store)
	})

	// ================================
	// SECURED API ROUTES
	// ================================

	secured := app.Group("/api")

	if !config.IsEnabled("demo") {
		secured.Use(func(c *fiber.Ctx) error {
			return middleware.JwtMiddleware(c, store)
		})
		logging.Info("JWT authentication enabled for API routes")
	} else {
		logging.Warn("Running in demo mode - API routes are UNSECURED!")
	}

	// Statistics routes
	secured.Get("/timeseries", func(c *fiber.Ctx) error {
		return statistics.GetProfilesTimeSeriesData(c, store)
	})

	secured.Get("/activitydata", func(c *fiber.Ctx) error {
		return statistics.GetMonthlyStorageStats(c, store)
	})

	secured.Get("/barchartdata", func(c *fiber.Ctx) error {
		return statistics.GetNotesMediaStorageData(c, store)
	})

	secured.Get("/kinds", func(c *fiber.Ctx) error {
		return statistics.GetKindData(c, store)
	})

	secured.Get("/kind-trend/:kindNumber", func(c *fiber.Ctx) error {
		return statistics.GetKindTrendData(c, store)
	})

	// Bitcoin routes
	secured.Post("/updateRate", func(c *fiber.Ctx) error {
		return bitcoin.UpdateBitcoinRate(c, store)
	})

	secured.Get("/bitcoin-rates/last-30-days", func(c *fiber.Ctx) error {
		return bitcoin.GetBitcoinRatesLast30Days(c, store)
	})

	// Wallet routes
	secured.Get("/balance/usd", func(c *fiber.Ctx) error {
		return wallet.GetWalletBalanceUSD(c, store)
	})

	secured.Get("/transactions/latest", func(c *fiber.Ctx) error {
		return wallet.GetLatestWalletTransactions(c, store)
	})

	secured.Get("/addresses", func(c *fiber.Ctx) error {
		return wallet.PullWalletAddresses(c, store)
	})

	secured.Post("/pending-transactions", func(c *fiber.Ctx) error {
		return wallet.SaveUnconfirmedTransaction(c, store)
	})

	secured.Post("/replacement-transactions", func(c *fiber.Ctx) error {
		return wallet.ReplaceTransaction(c, store)
	})

	secured.Get("/pending-transactions", func(c *fiber.Ctx) error {
		return wallet.GetPendingTransactions(c, store)
	})

	// Auth routes
	secured.Get("/paid-subscriber-profiles", func(c *fiber.Ctx) error {
		return handlers.HandleGetPaidSubscriberProfiles(c, store)
	})

	// Profiles route
	secured.Post("/profiles", func(c *fiber.Ctx) error {
		return handlers.HandleGetProfiles(c, store)
	})

	secured.Post("/refresh-token", func(c *fiber.Ctx) error {
		return auth.RefreshToken(c)
	})

	// General handlers routes
	secured.Get("/files", func(c *fiber.Ctx) error {
		return handlers.HandleGetFilesByType(c, store)
	})

	// Settings routes
	app.Get("/api/settings", settings.GetSettings)
	app.Post("/api/settings", func(c *fiber.Ctx) error {
		return settings.UpdateSettings(c, store)
	})

	// Individual setting routes (optional - for granular control)
	app.Get("/api/settings/:key", settings.GetSettingValue)
	app.Put("/api/settings/:key", settings.UpdateSettingValue)

	// Relay count route
	app.Get("/api/relay/count", func(c *fiber.Ctx) error {
		return settings.GetRelayCount(c, store)
	})

	// Relay icon upload route (secured with JWT)
	secured.Post("/relay/icon", func(c *fiber.Ctx) error {
		return settings.UploadRelayIcon(c, store)
	})

	// ================================
	// MODERATION ROUTES
	// ================================

	secured.Get("/moderation/notifications", func(c *fiber.Ctx) error {
		return moderation.GetModerationNotifications(c, store)
	})

	secured.Post("/moderation/notifications/read", func(c *fiber.Ctx) error {
		return moderation.MarkNotificationAsRead(c, store)
	})

	secured.Post("/moderation/notifications/read-all", func(c *fiber.Ctx) error {
		return moderation.MarkAllNotificationsAsRead(c, store)
	})

	secured.Get("/moderation/stats", func(c *fiber.Ctx) error {
		return moderation.GetModerationStats(c, store)
	})

	secured.Post("/moderation/notifications", func(c *fiber.Ctx) error {
		return moderation.CreateModerationNotification(c, store)
	})

	secured.Get("/moderation/blocked-event/:id", func(c *fiber.Ctx) error {
		return moderation.GetBlockedEvent(c, store)
	})

	secured.Post("/moderation/unblock", func(c *fiber.Ctx) error {
		return moderation.UnblockEvent(c, store)
	})

	secured.Delete("/moderation/event/:id", func(c *fiber.Ctx) error {
		return moderation.DeleteModeratedEvent(c, store)
	})

	secured.Get("/blocked-pubkeys", func(c *fiber.Ctx) error {
		return handlers.GetBlockedPubkeys(c, store)
	})

	secured.Post("/blocked-pubkeys", func(c *fiber.Ctx) error {
		return handlers.BlockPubkey(c, store)
	})

	secured.Delete("/blocked-pubkeys/:pubkey", func(c *fiber.Ctx) error {
		return handlers.UnblockPubkey(c, store)
	})

	// ================================
	// GENERAL HANDLER ROUTES
	// ================================

	// These are in the main handlers package
	secured.Get("/reports/notifications", func(c *fiber.Ctx) error {
		return handlers.GetReportNotifications(c, store)
	})

	secured.Post("/reports/notifications/read", func(c *fiber.Ctx) error {
		return handlers.MarkReportNotificationAsRead(c, store)
	})

	secured.Post("/reports/notifications/read-all", func(c *fiber.Ctx) error {
		return handlers.MarkAllReportNotificationsAsRead(c, store)
	})

	secured.Get("/reports/stats", func(c *fiber.Ctx) error {
		return handlers.GetReportStats(c, store)
	})

	secured.Get("/reports/event/:id", func(c *fiber.Ctx) error {
		return handlers.GetReportedEvent(c, store)
	})

	secured.Delete("/reports/event/:id", func(c *fiber.Ctx) error {
		return handlers.DeleteReportedEvent(c, store)
	})

	secured.Get("/payment/notifications", func(c *fiber.Ctx) error {
		return handlers.GetPaymentNotifications(c, store)
	})

	secured.Post("/payment/notifications/read", func(c *fiber.Ctx) error {
		return handlers.MarkPaymentNotificationAsRead(c, store)
	})

	secured.Post("/payment/notifications/read-all", func(c *fiber.Ctx) error {
		return handlers.MarkAllPaymentNotificationsAsRead(c, store)
	})

	secured.Get("/payment/stats", func(c *fiber.Ctx) error {
		return handlers.GetPaymentStats(c, store)
	})

	secured.Post("/payment/notifications", func(c *fiber.Ctx) error {
		return handlers.CreatePaymentNotification(c, store)
	})

	// Allowed users
	secured.Get("/allowed/users", func(c *fiber.Ctx) error {
		return access.GetAllowedUsersPaginated(c, store)
	})

	secured.Post("/allowed/add", func(c *fiber.Ctx) error {
		return access.AddAllowedUser(c, store)
	})

	secured.Delete("/allowed/remove", func(c *fiber.Ctx) error {
		return access.RemoveAllowedUser(c, store)
	})

	// Relay owner management
	secured.Get("/admin/owner", func(c *fiber.Ctx) error {
		return access.GetRelayOwner(c, store)
	})

	secured.Post("/admin/owner", func(c *fiber.Ctx) error {
		return access.SetRelayOwner(c, store)
	})

	secured.Delete("/admin/owner", func(c *fiber.Ctx) error {
		return access.RemoveRelayOwner(c, store)
	})

	// ================================
	// PUSH NOTIFICATION ROUTES
	// ================================

	secured.Post("/push/register", push.RegisterDeviceHandler(store))

	secured.Post("/push/unregister", push.UnregisterDeviceHandler(store))

	secured.Post("/push/test", push.TestNotificationHandler(store))

	// ================================
	// STATIC FILE SERVING
	// ================================
	app.Use(filesystem.New(filesystem.Config{
		Root:   http.Dir("./web"),
		Browse: false,
		Index:  "index.html",
		// Only serve static files, don't catch API routes or blossom routes
		Next: func(c *fiber.Ctx) bool {
			return strings.HasPrefix(c.Path(), "/api/") || strings.HasPrefix(c.Path(), "/blossom/")
		},
	}))

	// Catch-all for non-API and non-blossom routes only
	app.Use(func(c *fiber.Ctx) error {
		// Don't interfere with API routes or blossom routes
		if strings.HasPrefix(c.Path(), "/api/") || strings.HasPrefix(c.Path(), "/blossom/") {
			return c.Next()
		}
		return c.SendFile("./web/index.html")
	})

	// ================================
	// BACKGROUND SERVICES
	// ================================
	go services.PullBitcoinPrice(store)

	// Start the server
	port := config.GetPort("web")
	address := fmt.Sprintf("%s:%d", viper.GetString("server.bind_address"), port)

	logging.Info("Web server starting", map[string]interface{}{
		"address": address,
	})

	//if viper.GetBool("server.upnp") {
	//	upnp := upnp.Get()
	//
	//	err := upnp.ForwardPort(uint16(port), "Hornet Storage Web Panel")
	//	if err != nil {
	//		logging.Error("Failed to forward port using UPnP", map[string]interface{}{
	//			"port": port,
	//		})
	//	}
	//}

	return app.Listen(address)
}
