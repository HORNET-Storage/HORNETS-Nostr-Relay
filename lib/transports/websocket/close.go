package websocket

import (
	"fmt"
	"log"

	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"
)

func handleCloseMessage(c *websocket.Conn, env *nostr.CloseEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var closeEvent []string
	err := json.Unmarshal([]byte(env.String()), &closeEvent)
	if err != nil {
		fmt.Println("Error:", err)
		errMsg := "Error unmarshalling CLOSE request: " + err.Error()
		if writeErr := sendWebSocketMessage(c, nostr.NoticeEnvelope(errMsg)); writeErr != nil {
			fmt.Println("Error sending NOTICE message:", writeErr)
		}
	}
	subscriptionID := closeEvent[1]

	removeListenerId(c, subscriptionID)

	responseMsg := nostr.ClosedEnvelope{SubscriptionID: subscriptionID, Reason: "Subscription closed successfully."}

	if err := sendWebSocketMessage(c, responseMsg); err != nil {
		log.Printf("Error sending 'CLOSED' envelope over WebSocket: %v", err)
	}
}
