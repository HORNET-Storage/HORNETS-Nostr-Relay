package wallet

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/spf13/viper"
)

// satoshiToUSD calculates the USD value of a given amount of Satoshis based on the current Bitcoin rate and rounds it to two decimal places.
func SatoshiToUSD(bitcoinRate float64, satoshis int64) float64 {
	// One Bitcoin is 100,000,000 Satoshis.
	const satoshisPerBitcoin = 100000000

	// Calculate the value of one Satoshi in USD.
	satoshiValueInUSD := bitcoinRate / float64(satoshisPerBitcoin)

	// Calculate the total USD value of the given Satoshis.
	totalUSD := float64(satoshis) * satoshiValueInUSD

	// Round the result to two decimal places.
	roundedTotalUSD := math.Round(totalUSD*100) / 100

	return roundedTotalUSD
}

// extractTransactionDetails parses and validates transaction data
func ExtractTransactionDetails(transaction map[string]interface{}) (*transactionDetails, error) {
	address, ok := transaction["address"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid address")
	}

	dateStr, ok := transaction["date"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid date")
	}
	date, err := time.Parse(time.RFC3339, dateStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing date: %v", err)
	}

	output, ok := transaction["output"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid output")
	}

	valueStr, ok := transaction["value"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid value")
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return nil, fmt.Errorf("error parsing value: %v", err)
	}

	return &transactionDetails{
		address:  address,
		date:     date,
		output:   output,
		value:    value,
		valueStr: valueStr,
	}, nil
}

// validateWalletName ensures the wallet name is valid and consistent
func ValidateWalletName(transactions []map[string]interface{}) string {
	expectedWalletName := viper.GetString("wallet.name")

	// Set wallet name from first transaction if not set
	if expectedWalletName == "" && len(transactions) > 0 {
		if walletName, ok := transactions[0]["wallet_name"].(string); ok {
			// Use UpdateConfig with save=false for runtime-only value
			// This prevents pollution of viper memory that would be saved later
			config.UpdateConfig("wallet.name", walletName, false)
			expectedWalletName = walletName
		}
	}

	return expectedWalletName
}
