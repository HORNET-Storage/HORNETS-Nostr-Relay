// credit.go - Credit system management

package subscription

import (
	"fmt"
	"log"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// checkAndApplyCredit checks if the subscriber's credit can be used to purchase any tier
// and applies it if possible. It returns the remaining credit and any error.
func (m *SubscriptionManager) checkAndApplyCredit(
	npub string,
	address string,
	storageInfo *StorageInfo,
	endDate time.Time,
) (int64, error) {
	// Get current credit
	credit, err := m.store.GetStatsStore().GetSubscriberCredit(npub)
	if err != nil {
		return 0, fmt.Errorf("failed to get subscriber credit: %v", err)
	}

	// If credit is too low, just return it
	if credit <= 0 {
		return credit, nil
	}

	log.Printf("Checking if credit of %d sats can be used for any tier", credit)

	// Try to find a tier that the credit can afford
	tier, err := m.findMatchingTier(credit)
	if err != nil {
		// No matching tier, just return the credit
		log.Printf("No tier found for credit of %d sats", credit)
		return credit, nil
	}

	// We found a tier! Apply it
	tierPriceSats := int64(tier.PriceSats)
	tierBytes := tier.MonthlyLimitBytes
	if tier.Unlimited {
		tierBytes = types.MaxMonthlyLimitBytes
	}

	// Add storage
	prevBytes := storageInfo.TotalBytes
	storageInfo.TotalBytes += tierBytes
	storageInfo.UpdatedAt = time.Now()

	log.Printf("Using credit of %d sats for tier: %s (adding %d bytes to existing %d bytes, new total: %d bytes)",
		tierPriceSats, tier.Name, tierBytes, prevBytes, storageInfo.TotalBytes)

	// Update the NIP-88 event
	err = m.createOrUpdateNIP88Event(&types.Subscriber{
		Npub:    npub,
		Address: address,
	}, tier, endDate, storageInfo)

	if err != nil {
		return credit, fmt.Errorf("failed to update NIP-88 event with credit-purchased tier: %v", err)
	}

	// Update paid subscriber record
	m.updatePaidSubscriberRecord(npub, tier, endDate, storageInfo)

	// Update credit in database
	remainingCredit := credit - tierPriceSats
	if err := m.store.GetStatsStore().UpdateSubscriberCredit(npub, remainingCredit); err != nil {
		return remainingCredit, fmt.Errorf("failed to update credit after using for tier: %v", err)
	}

	log.Printf("Successfully used %d sats from credit for tier %s, remaining credit: %d",
		tierPriceSats, tier.Name, remainingCredit)

	// Check if remaining credit can be used for another tier recursively
	if remainingCredit > 0 {
		return m.checkAndApplyCredit(npub, address, storageInfo, endDate)
	}

	return remainingCredit, nil
}
