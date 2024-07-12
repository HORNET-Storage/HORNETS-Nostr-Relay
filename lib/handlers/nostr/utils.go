package nostr

import (
	"fmt"
	"log"
	"strconv"

	"time"

	"github.com/fxamacker/cbor/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
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

	// Load and check relay settings
	settings, err := LoadRelaySettings()
	if err != nil {
		write("NOTICE", "Failed to load relay settings")
		return false
	}

	// Check if the event kind is allowed
	blocked := IsTheKindAllowed(env.Event.Kind, settings)
	if !blocked {
		write("OK", env.Event.ID, false, "This kind is not handled by the relay")
		return false
	}

	timeCheck := TimeCheck(env.Event.CreatedAt.Time().Unix())
	if !timeCheck {
		write("OK", env.Event.ID, false, "The event creation date must be after January 1, 2019")
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

// Returns true if the event timestamp is valid, or false with an error message if it's too far off.
func TimeCheck(eventCreatedAt int64) bool {
	thresholdDate := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	eventTime := time.Unix(eventCreatedAt, 0)

	// Check if the event timestamp is before January 1, 2019
	return !eventTime.Before(thresholdDate)
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
		log.Printf("Error marshaling response message: %s\n", err)
		return
	}

	// Write the JSON message to the stream
	if _, err := stream.Write(jsonMessage); err != nil {
		log.Printf("Error writing to stream: %s\n", err)
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
		log.Printf("Error marshaling response message: %s\n", err)
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
		log.Printf("Error marshaling response message: %s\n", err)
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
		log.Printf("Error closing stream: %s\n", err)
	}
}

func LoadRelaySettings() (*types.RelaySettings, error) {
	viper.SetConfigName("config") // Name of config file (without extension)
	viper.SetConfigType("json")   // Type of the config file
	viper.AddConfigPath(".")      // Path to look for the config file in

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %s", err)
		return nil, err
	}

	var settings types.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
		log.Fatalf("Error unmarshaling config into struct: %s (nostr/utils)", err)
		return nil, err
	}

	return &settings, nil
}

func IsTheKindAllowed(kind int, settings *types.RelaySettings) bool {
	if settings.Mode != "smart" {
		return true
	}

	kindStr := "kind" + strconv.Itoa(kind)
	for _, k := range settings.Kinds {
		if k == kindStr {
			return true
		}
	}

	kindStrWithoutPrefix := strconv.Itoa(kind)
	for _, dk := range settings.DynamicKinds {
		if dk == kindStrWithoutPrefix {
			return true
		}
	}

	return false
}

func IsKindBlocked(kind int, settings *types.RelaySettings) bool {
	kindStr := "kind" + strconv.Itoa(kind)
	kindStrWithoutPrefix := strconv.Itoa(kind)

	if settings.Mode == "unlimited" {
		for _, k := range settings.Kinds {
			if k == kindStr {
				return true
			}
		}
		for _, dk := range settings.DynamicKinds {
			if dk == kindStrWithoutPrefix {
				return true
			}
		}
		return false
	}

	return true // Default: allow all kinds if mode is not specified
}
