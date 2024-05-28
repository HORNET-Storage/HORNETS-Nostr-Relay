package web

import (
	"math"
)

// satoshiToUSD calculates the USD value of a given amount of Satoshis based on the current Bitcoin rate and rounds it to two decimal places.
func satoshiToUSD(bitcoinRate float64, satoshis int64) float64 {
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
