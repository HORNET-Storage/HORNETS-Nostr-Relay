package main

import (
	"fmt"
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/web"
	"github.com/spf13/viper"

	//"github.com/libp2p/go-libp2p/p2p/security/noise"
	//libp2ptls "github.com/libp2p/go-libp2p/p2p/security/tls"
	//stores_bbolt "github.com/HORNET-Storage/hornet-storage/lib/stores/bbolt"
	//stores_memory "github.com/HORNET-Storage/hornet-storage/lib/stores/memory"
	//negentropy "github.com/illuzen/go-negentropy"

	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
)

func init() {
	viper.SetDefault("key", "")
	viper.SetDefault("web", true)
	viper.SetDefault("proxy", true)
	viper.SetDefault("port", "9000")
	viper.SetDefault("relay_stats_db", "relay_stats.db")
	viper.SetDefault("service_tag", "hornet-storage-service")

	viper.AddConfigPath(".")
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}
}

func main() {
	store := &stores_graviton.GravitonStore{}

	queryCache := viper.GetStringMapString("query_cache")
	err := store.InitStore("gravitondb", queryCache)
	if err != nil {
		log.Fatal(err)
	}

	statDb, err := stores_graviton.InitGorm()
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return
	}

	err = web.StartServer(store, statDb)

	if err != nil {
		fmt.Println("Fatal error occurred in web server")
	}
}
