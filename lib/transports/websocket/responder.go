package websocket

import (
	"encoding/json"
	"log"

	"github.com/gofiber/contrib/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
)

func sendWebSocketMessage(ws *websocket.Conn, msg interface{}) error {
	// msg is any of nostr.ClosedEnvelope, nostr.EOSEEnvelope, nostr.OKEnvelope, nostr.EventEnvelope, nostr.NoticeEnvelope
	marshalledMsg, err := jsoniter.MarshalToString(msg)
	if err != nil {
		log.Println()
	}
	log.Println("Websocket message: ", marshalledMsg)
	if err := ws.WriteJSON(msg); err != nil {
		log.Printf("Error sending message over WebSocket: %v", err)
		return err
	}
	return nil
}

// Example of how you might parse and handle incoming messages to determine
// the appropriate nostr envelope to send over WebSocket.
func handleIncomingMessage(ws *websocket.Conn, jsonMessage []byte) {
	// Unmarshal the JSON array into a slice of interfaces
	var messageSlice []interface{}
	err := jsoniter.ConfigCompatibleWithStandardLibrary.Unmarshal(jsonMessage, &messageSlice)
	if err != nil {
		log.Printf("Error unmarshaling incoming message: %s", err)
		return
	}

	// Ensure the message has at least 2 elements (type and subID)
	if len(messageSlice) < 2 {
		log.Println("Incoming message does not have the expected format or number of elements.")
		return
	}

	// Extract the message type and subscription ID
	messageType, ok := messageSlice[0].(string)
	if !ok {
		log.Println("First element of the message is not a string indicating the type.")
		return
	}
	subID, ok := messageSlice[1].(string)
	if !ok {
		log.Println("Second element of the message is not a string indicating the subscription ID.")
		return
	}

	// Handle the message based on its type
	switch messageType {
	case "EOSE":
		// EOSE does not need additional data beyond subID, which is already extracted
		sendWebSocketMessage(ws, nostr.EOSEEnvelope(subID))
	case "EVENT":
		// For "EVENT", assuming direct JSON structure for the event details as the third element
		if len(messageSlice) < 3 {
			log.Println("Expected data for 'EVENT' message type is missing.")
			return
		}
		eventDataJSON, ok := messageSlice[2].(string) // Assuming eventData is provided as a JSON string
		if !ok {
			log.Println("Expected data for 'EVENT' message type is not a string.")
			return
		}
		var eventData nostr.Event
		if err := json.Unmarshal([]byte(eventDataJSON), &eventData); err != nil {
			log.Printf("Error unmarshalling 'event' data: %s", err)
			return
		}
		sendWebSocketMessage(ws, nostr.EventEnvelope{SubscriptionID: &subID, Event: eventData})
	case "NOTICE":
		// Assuming "NOTICE" message contains a string as the third element
		if len(messageSlice) < 2 {
			log.Println("Expected data for 'NOTICE' message type is missing.")
			return
		}
		noticeMsg, ok := messageSlice[1].(string)
		if !ok {
			log.Println("Expected data for 'NOTICE' message type is not a string.")
			return
		}
		sendWebSocketMessage(ws, nostr.NoticeEnvelope(noticeMsg))
	case "OK":
		// Assuming the OK message includes the event ID as the second element and a boolean as the third.
		if len(messageSlice) < 3 {
			log.Println("Expected data for 'OK' message type is missing.")
			return
		}
		eventID, ok := messageSlice[1].(string) // Correctly extracting the event ID
		if !ok {
			log.Println("Expected event ID for 'OK' message type is not a string.")
			return
		}

		// Correctly assuming the third element is a boolean indicating success.
		success, ok := messageSlice[2].(bool)
		if !ok {
			log.Println("Expected success indicator for 'OK' message type is not a boolean.")
			return
		}

		// Check if a specific reason was provided as the fourth element
		var reason string
		if len(messageSlice) > 3 {
			if reasonStr, ok := messageSlice[3].(string); ok {
				reason = reasonStr
			}
		}

		// If no reason was provided but success is false, use a generic message
		if !success && reason == "" {
			reason = "Operation failed - Check server logs for details"
		}

		// Constructing the OKEnvelope with the provided data.
		okEnvelope := nostr.OKEnvelope{
			EventID: eventID,
			OK:      success,
			Reason:  reason, // Note: This will be empty if success is true, based on current message format.
		}
		// Sending the constructed OKEnvelope.
		sendWebSocketMessage(ws, okEnvelope)

	case "COUNT":
		type CountStruct struct {
			Count int `json:"count"`
		}
		// Assuming the COUNT message includes the subscription ID as the second element
		if len(messageSlice) < 2 {
			log.Println("Expected data for 'COUNT' message type is missing.")
			return
		}
		countMsg, ok := messageSlice[2].(string)
		if !ok {
			log.Println("Expected data for 'COUNT' message type is not a string.")
			return
		}
		var count CountStruct
		err := json.Unmarshal([]byte(countMsg), &count)
		if err != nil {
			log.Println("Error:", err)
			return
		}
		if err := sendWebSocketMessage(ws, messageSlice); err != nil {
			log.Printf("Error sending 'COUNT' envelope over WebSocket: %v", err)
		}

	case "AUTH":
		if len(messageSlice) < 2 {
			log.Println("Expected data for 'AUTH' message type is missing.")
			return
		}
		challengeString, ok := messageSlice[1].(string)
		if !ok {
			log.Println("Expected challenge string for 'AUTH' message type is not a string.")
			return
		}
		log.Println("Dealing with auth message message")
		// Send the AUTH message with the signed event
		authEnvelope := nostr.AuthEnvelope{
			Challenge: &challengeString,
		}

		sendWebSocketMessage(ws, authEnvelope)

	default:
		log.Printf("Unhandled message type: %s", messageType)
	}
}
