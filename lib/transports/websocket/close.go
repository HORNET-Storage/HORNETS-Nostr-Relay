package websocket

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	jsoniter "github.com/json-iterator/go"

	"github.com/gofiber/contrib/websocket"
	"github.com/nbd-wtf/go-nostr"
)

func handleCloseMessage(c *websocket.Conn, env *nostr.CloseEnvelope) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var closeEvent []string
	err := json.Unmarshal([]byte(env.String()), &closeEvent)
	if err != nil {
		logging.Infof("Error:%s", err)
		errMsg := "Error unmarshalling CLOSE request: " + err.Error()
		if writeErr := sendWebSocketMessage(c, nostr.NoticeEnvelope(errMsg)); writeErr != nil {
			logging.Infof("Error sending NOTICE message:%s", writeErr)
		}
	}
	subscriptionID := closeEvent[1]

	removeListenerId(c, subscriptionID)

	responseMsg := nostr.ClosedEnvelope{SubscriptionID: subscriptionID, Reason: "Subscription closed successfully."}

	if err := sendWebSocketMessage(c, responseMsg); err != nil {
		logging.Infof("Error sending 'CLOSED' envelope over WebSocket: %v", err)
	}
}
