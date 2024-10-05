package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/stats_stores"
	"github.com/spf13/viper"
)

type CoinGeckoResponse struct {
	Bitcoin struct {
		USD float64 `json:"usd"`
	} `json:"bitcoin"`
}

type BinanceResponse struct {
	Price string `json:"price"`
}

type MempoolResponse struct {
	USD float64 `json:"USD"`
}

func fetchCoinGeckoPrice() (float64, error) {
	resp, err := http.Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var result CoinGeckoResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}
	return result.Bitcoin.USD, nil
}

func fetchBinancePrice() (float64, error) {
	resp, err := http.Get("https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// If the response status is not 200, return an error
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("binance API returned non-OK status: %v", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var result BinanceResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}

	price, err := strconv.ParseFloat(result.Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}

func fetchMempoolPrice() (float64, error) {
	resp, err := http.Get("https://mempool.space/api/v1/prices")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var result MempoolResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}
	return result.USD, nil
}

func fetchBitcoinPrice(apiIndex int) (float64, int, error) {
	apis := []func() (float64, error){
		fetchCoinGeckoPrice,
		fetchBinancePrice,
		fetchMempoolPrice,
	}
	for i := 0; i < len(apis); i++ {
		index := (apiIndex + i) % len(apis)
		price, err := apis[index]()
		if err == nil {
			return price, (index + 1) % len(apis), nil
		}
		fmt.Println("Error fetching price from API", index, ":", err)
	}
	return 0, apiIndex, fmt.Errorf("all API calls failed")
}

func pullBitcoinPrice() {
	// Fetch the initial Bitcoin rate immediately
	store := &gorm.GormStatisticsStore{}
	err := store.InitStore(viper.GetString("relay_stats_db"), nil) // Provide the correct path to your SQLite DB here
	if err != nil {
		fmt.Println("Error initializing database:", err)
		return
	}
	apiIndex := 0
	price, newIndex, err := fetchBitcoinPrice(apiIndex)
	if err != nil {
		fmt.Println("Error:", err)
	} else {
		fmt.Printf("Initial Bitcoin Price from APIs: $%.2f\n", price)
		apiIndex = newIndex
		store.SaveBitcoinRate(price)
	}

	// Set up the ticker for subsequent fetches
	ticker := time.NewTicker(7 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		price, newIndex, err := fetchBitcoinPrice(apiIndex)
		if err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Printf("Bitcoin Price from APIs: $%.2f\n", price)
			apiIndex = newIndex
			store.SaveBitcoinRate(price)
		}
	}
}
