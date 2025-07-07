package kind1984

import (
	jsoniter "github.com/json-iterator/go"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// BuildKind1984Handler constructs and returns a handler function for kind 1984 (Report) events.
func BuildKind1984Handler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	handler := func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary

		// Read data from the stream.
		data, err := read()
		if err != nil {
			write("NOTICE", "Error reading from stream.")
			return
		}

		// Unmarshal the received data into a Nostr event
		var env nostr.EventEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			write("NOTICE", "Error unmarshaling event.")
			return
		}

		// Check relay settings for allowed events whilst also verifying signatures and kind number
		success := lib_nostr.ValidateEvent(write, env, 1984)
		if !success {
			return
		}

		// Validate the report event's tags.
		reportedEventID, reportedPubkey, reportType, errMsg := validateAndExtractReportData(env.Event.Tags)
		if errMsg != "" {
			write("OK", env.Event.ID, false, errMsg)
			return
		}

		// If an event ID is provided, verify that the event exists in our database
		if reportedEventID != "" {
			filter := nostr.Filter{
				IDs: []string{reportedEventID},
			}
			events, err := store.QueryEvents(filter)
			if err != nil || len(events) == 0 {
				write("OK", env.Event.ID, false, "Reported event not found in our database")
				return
			}

			// Check if the event is already blocked or pending moderation
			isBlocked, _ := store.IsEventBlocked(reportedEventID)
			isPending, _ := store.IsPendingModeration(reportedEventID)
			if isBlocked || isPending {
				// We acknowledge the report but don't create a notification since
				// the event is already being handled by moderation
				write("OK", env.Event.ID, true, "Event already being processed by moderation")

				// Still store the report event
				if err := store.StoreEvent(&env.Event); err != nil {
					write("NOTICE", "Failed to store the report event")
					return
				}
				return
			}

			// Try to find an existing report notification for this event
			existingNotification, err := store.GetStatsStore().GetReportNotificationByEventID(reportedEventID)
			if err != nil {
				logging.Infof("Error checking for existing report notification: %v", err)
				// Continue anyway, as we'll just try to create a new one
			}

			if existingNotification != nil {
				// Increment the report count
				err = store.GetStatsStore().UpdateReportCount(reportedEventID)
				if err != nil {
					logging.Infof("Error updating report count: %v", err)
				}
			} else {
				// Create a new report notification
				notification := &lib.ReportNotification{
					PubKey:         reportedPubkey,
					EventID:        reportedEventID,
					ReportType:     reportType,
					ReportContent:  env.Event.Content,
					ReporterPubKey: env.Event.PubKey,
					ReportCount:    1,
					// CreatedAt handled by GORM
					IsRead: false,
				}

				err = store.GetStatsStore().CreateReportNotification(notification)
				if err != nil {
					logging.Infof("Error creating report notification: %v", err)
				}
			}
		}

		// Store the report event
		if err := store.StoreEvent(&env.Event); err != nil {
			write("NOTICE", "Failed to store the report event")
			return
		}

		// Successfully processed event
		write("OK", env.Event.ID, true, "Report processed successfully")
	}

	return handler
}

// validateAndExtractReportData checks if the tags array contains the expected structure
// and extracts the reported event ID, pubkey and report type.
func validateAndExtractReportData(tags nostr.Tags) (string, string, string, string) {
	var reportedEventID, reportedPubkey, reportType string

	// Look for valid report tags
	for _, tag := range tags {
		if tag[0] == "e" && len(tag) >= 3 {
			reportedEventID = tag[1]
			reportType = tag[2]
		} else if tag[0] == "p" && len(tag) >= 2 {
			reportedPubkey = tag[1]
			if len(tag) >= 3 {
				// If there's a third element, it's the report type
				reportType = tag[2]
			}
		}
	}

	// Need at least a pubkey or an event ID
	if reportedEventID == "" && reportedPubkey == "" {
		return "", "", "", "Report event missing valid 'p' or 'e' report tag."
	}

	// Need a report type
	if reportType == "" {
		return reportedEventID, reportedPubkey, "", "Report event missing report type in tag."
	}

	return reportedEventID, reportedPubkey, reportType, ""
}
