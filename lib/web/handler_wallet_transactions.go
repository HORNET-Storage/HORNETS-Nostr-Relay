package web

import (
	"fmt"
	"log"
	"strconv"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

func getLatestWalletTransactions(c *fiber.Ctx, store stores.Store) error {
	// Get the latest wallet transactions
	transactions, err := store.GetStatsStore().GetLatestWalletTransactions()
	if err != nil {
		log.Printf("Error querying transactions: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Database query error",
		})
	}

	// Process each transaction to convert the value to USD
	for i, transaction := range transactions {
		value, err := strconv.ParseFloat(transaction.Value, 64)
		if err != nil {
			log.Printf("Error converting value to float64: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Conversion error",
			})
		}
		// You can adjust the format as needed, currently keeping the value as satoshis
		transactions[i].Value = fmt.Sprintf("%.8f", value)
	}

	// Respond with the transactions
	return c.JSON(transactions)
}
