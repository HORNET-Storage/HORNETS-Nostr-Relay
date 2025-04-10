package main

import (
	"fmt"
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/HORNET-Storage/hornet-storage/lib/web"
	"github.com/spf13/viper"
	//"github.com/libp2p/go-libp2p/p2p/security/noise"
	//libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	//stores_bbolt "github.com/HORNET-Storage/hornet-storage/lib/stores/bbolt"
	//stores_memory "github.com/HORNET-Storage/hornet-storage/lib/stores/memory"
	//negentropy "github.com/illuzen/go-negentropy"
)

func init() {
	viper.SetDefault("key", "")
	viper.SetDefault("web", true)
	viper.SetDefault("proxy", false) // No need for websocket proxy in demo mode
	viper.SetDefault("port", "9000")
	viper.SetDefault("demo_mode", true) // Enable demo mode by default for the demo server
	viper.SetDefault("relay_stats_db", "demo_relay_stats.db")
	viper.SetDefault("service_tag", "hornet-storage-service-demo")
	viper.SetDefault("RelayName", "HORNETS DEMO")
	viper.SetDefault("RelayDescription", "DEMO RELAY - For demonstration purposes only")
	viper.SetDefault("RelayContact", "demo@hornets.net")
	viper.SetDefault("RelayVersion", "0.0.1-demo")

	viper.AddConfigPath(".")
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}

	// Always force demo mode to true for the demo server
	// This ensures demo mode is enabled regardless of config.json settings
	viper.Set("demo_mode", true)
}

func main() {
	log.Println("========================================")
	log.Println("  HORNETS RELAY DEMO MODE")
	log.Println("  Authentication bypassed for admin panel")
	log.Println("  For demonstration purposes only")
	log.Println("  NOT FOR PRODUCTION USE")
	log.Println("========================================")

	// Use a separate data directory for the demo server to avoid conflicts
	// Initialize BadgerHold store with a separate data directory for demo mode
	store, err := badgerhold.InitStore("demo-data")
	if err != nil {
		log.Fatal(err)
	}

	// Switch to using a separate statistics database for demo mode
	// This ensures we don't mix demo data with production statistics
	if err := store.UseDemoStatisticsDB(); err != nil {
		log.Printf("Warning: Failed to switch to demo statistics database: %v", err)
		log.Println("Continuing with standard statistics database...")
		// Continue anyway as this is not a critical failure
	} else {
		log.Println("Demo server is using a separate statistics database (demo_statistics.db)")
		log.Println("You can pre-populate this database with dummy data for demonstration purposes")
	}

	// Set up cleanup on exit
	defer func() {
		log.Println("Cleaning up demo relay resources...")
		err := store.Cleanup()
		if err != nil {
			log.Printf("Failed to cleanup demo data: %v", err)
		} else {
			log.Println("Demo data cleanup successful")
		}
	}()

	// Use a different port for the demo server to avoid conflicts
	demoPortStr := viper.GetString("port")
	var portNum int
	_, err = fmt.Sscanf(demoPortStr, "%d", &portNum)
	if err == nil && portNum > 0 {
		// If we got a port number successfully, add 1000 to avoid conflicting with main relay
		newPort := portNum + 1000
		viper.Set("port", fmt.Sprintf("%d", newPort))
		log.Printf("Demo server will use port %d (web panel on port %d)", newPort, newPort+2)
	}

	log.Println("Starting demo web server...")
	err = web.StartServer(store)

	if err != nil {
		log.Fatalf("Fatal error occurred in demo web server: %v", err)
	}
}
