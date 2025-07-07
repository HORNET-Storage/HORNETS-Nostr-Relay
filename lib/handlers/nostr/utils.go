package nostr

import (
	"fmt"

	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/fxamacker/cbor/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/nbd-wtf/go-nostr"
)

// Gerneric event validation that almost all kinds will use
func ValidateEvent(write KindWriter, env nostr.EventEnvelope, expectedKind int) bool {
	// If the expected kind is greater than -1 then we ensure the event kind matches the expected kind
	if expectedKind > -1 {
		if env.Event.Kind != expectedKind {
			write("OK", env.Event.ID, false, "Invalid event kind")
			return false
		}
	}

	// Check if the event kind is allowed
	blocked := IsKindAllowed(env.Event.Kind)
	if !blocked {
		write("OK", env.Event.ID, false, "This kind is not handled by the relay")
		return false
	}

	timeCheck := TimeCheck(env.Event.CreatedAt.Time().Unix())
	if !timeCheck {
		write("OK", env.Event.ID, false, "The event creation date must be after January 1, 2019")
		return false
	}

	// Validate the event signature
	success, err := env.Event.CheckSignature()
	if err != nil {
		write("NOTICE", "Failed to check signature")
		return false
	}

	if !success {
		write("OK", env.Event.ID, false, "Signature failed to verify")
		return false
	}

	return true
}

// Check if the event is pretending it can time travel
func TimeCheck(eventCreatedAt int64) bool {
	currentTime := time.Now()

	return eventCreatedAt <= currentTime.Add(2*time.Second).Unix()
}

func AuthTimeCheck(eventCreatedAt int64) (bool, string) {
	currentTime := time.Now()
	eventTime := time.Unix(eventCreatedAt, 0)
	tenMinutesAgo := currentTime.Add(-10 * time.Minute)

	// Check if the event timestamp is within the last 10 minutes
	if eventTime.Before(tenMinutesAgo) {
		errMsg := fmt.Sprintf("invalid: event creation date is more than 10 minutes ago (%s)", eventTime)
		return false, errMsg
	}

	return true, ""
}

// responder sends a response string through the given network stream
func Responder(stream network.Stream, messageType string, params ...interface{}) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	var message []interface{}
	message = append(message, messageType)
	message = append(message, params...)
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		logging.Infof("Error marshaling response message: %s\n", err)
		return
	}

	// Write the JSON message to the stream
	if _, err := stream.Write(jsonMessage); err != nil {
		logging.Infof("Error writing to stream: %s\n", err)
	}
}

func BuildResponse(messageType string, params ...interface{}) []byte {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	// Extract and flatten values from params before appending to the message
	extractedParams := extractInterfaceValues(params...)

	var message []interface{}
	message = append(message, messageType)
	// Append the extracted parameters individually to ensure a flat structure
	message = append(message, extractedParams...)

	jsonMessage, err := json.Marshal(message)
	if err != nil {
		logging.Infof("Error marshaling response message: %s\n", err)
		return nil
	}

	// Append a newline character to the JSON message to act as a delimiter
	jsonMessageWithDelimiter := append(jsonMessage, '\n')

	return jsonMessageWithDelimiter
}

func BuildCborResponse(messageType string, params ...interface{}) []byte {
	// Extract and flatten values from params before appending to the message
	extractedParams := extractInterfaceValues(params...)

	var message []interface{}
	message = append(message, messageType)
	// Append the extracted parameters individually to ensure a flat structure
	message = append(message, extractedParams...)

	cborMessage, err := cbor.Marshal(message)
	if err != nil {
		logging.Infof("Error marshaling response message: %s\n", err)
		return nil
	}

	return cborMessage
}

func extractInterfaceValues(data ...interface{}) []interface{} {
	var extracted []interface{}
	for _, v := range data {
		switch element := v.(type) {
		case []interface{}:
			// Recursively flatten nested slices
			extracted = append(extracted, extractInterfaceValues(element...)...)
		default:
			extracted = append(extracted, element)
		}
	}
	return extracted
}

func CloseStream(stream network.Stream) {
	if err := stream.CloseWrite(); err != nil {
		logging.Infof("Error closing stream: %s\n", err)
	}
}

func IsKindAllowed(kind int) bool {
	settings, err := config.GetConfig()
	if err != nil {
		return false
	}

	// Format the kind number to match the whitelist format
	kindStr := fmt.Sprintf("kind%d", kind)

	if len(settings.EventFiltering.KindWhitelist) > 0 {
		if !contains(settings.EventFiltering.KindWhitelist, kindStr) {
			return false
		}
	}

	return true
}

func contains(list []string, item string) bool {
	for _, element := range list {
		if element == item {
			return true
		}
	}
	return false
}
