package relaycount

import (
	"fmt"
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"

	lib_nostr "github.com/HORNET-Storage/hornet-storage/lib/handlers/nostr"
)

// This handler will handler data pulls for the web panel charts, however it may need to be modified due to the new structure for charts.
func BuildRelayCountsHandler(store stores.Store) func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
	return func(read lib_nostr.KindReader, write lib_nostr.KindWriter) {
		var json = jsoniter.ConfigCompatibleWithStandardLibrary
		log.Println("Working with relay count request.")

		data, err := read()
		if err != nil {
			log.Println("Error reading from stream:", err)
			write("NOTICE", "Error reading from stream.")
			return
		}

		var kindData map[string][]int
		if err := json.Unmarshal(data, &kindData); err != nil {
			log.Println("Error unmarshaling data:", err)
			write("NOTICE", "Error unmarshaling data.")
			return
		}

		responseData := make(map[string]int)

		for _, kinds := range kindData {
			for _, kind := range kinds {
				filter := nostr.Filter{
					Kinds: []int{kind},
				}

				count, err := store.QueryEvents(filter)
				if err != nil {
					log.Printf("Error querying events for kind %d: %v", kind, err)
					continue
				}

				// Properly converting kind from int to string
				responseData[fmt.Sprint(kind)] += len(count)
			}
		}

		log.Printf("Response Data: %+v", responseData)
		responseJSON, err := json.Marshal(responseData)
		if err != nil {
			log.Println("Error marshaling response data:", err)
			write("ERROR", "Error marshaling response data.")
			return
		}

		log.Println(string(responseJSON))
		write(string(responseJSON))
	}
}
