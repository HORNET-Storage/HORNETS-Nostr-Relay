package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"math/rand"
)

func init() {
	viper.SetDefault("web", false)
	viper.SetDefault("proxy", true)
	viper.SetDefault("port", "9000")
	viper.SetDefault("relay_stats_db", "relay_stats.db")
	viper.SetDefault("query_cache", map[string]string{})
	viper.SetDefault("service_tag", "hornet-storage-service")
	viper.SetDefault("relay_name", fmt.Sprintf("hornet-storage-%d", rand.Intn(1000000)))
	viper.SetDefault("key", "")
	viper.SetDefault("relay_pub_key", "")
	viper.SetDefault("supported_nips", []int{0, 1, 3, 5, 6, 7, 8, 97, 1984, 9735, 9372, 9373, 9802, 10000, 10001, 10002, 30000, 30008, 30009, 30023, 30079})

	viper.AddConfigPath(".")
	viper.SetConfigType("json")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			viper.SafeWriteConfig()
		}
	}

	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
	})

	viper.WatchConfig()
}

func main() {
}
