package nostr

import (
	"fmt"
	"log"
	"strconv"

	"time"

	"github.com/fxamacker/cbor/v2"
	jsoniter "github.com/json-iterator/go"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/spf13/viper"
)

// Returns true if the event timestamp is valid, or false with an error message if it's too far off.
func TimeCheck(eventCreatedAt int64) (bool, string) {
	thresholdDate := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	eventTime := time.Unix(eventCreatedAt, 0)

	// Check if the event timestamp is before January 1, 2019
	if eventTime.Before(thresholdDate) {
		errMsg := fmt.Sprintf("invalid: event creation date is before January 1, 2019 (%s)", eventTime)
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

	log.Println("Writing to stream:", string(jsonMessage))

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

	log.Println("Checking how message looks.", message)

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

	log.Println("Checking how message looks.", message)

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

type RelaySettings struct {
	Mode             string   `json:"mode"`
	Protocol         []string `json:"protocol"`
	Chunked          []string `json:"chunked"`
	Chunksize        string   `json:"chunksize"`
	MaxFileSize      int      `json:"maxFileSize"`
	MaxFileSizeUnit  string   `json:"maxFileSizeUnit"`
	Kinds            []string `json:"kinds"`
	DynamicKinds     []string `json:"dynamicKinds"`
	Photos           []string `json:"photos"`
	Videos           []string `json:"videos"`
	GitNestr         []string `json:"gitNestr"`
	Audio            []string `json:"audio"`
	IsKindsActive    bool     `json:"isKindsActive"`
	IsPhotosActive   bool     `json:"isPhotosActive"`
	IsVideosActive   bool     `json:"isVideosActive"`
	IsGitNestrActive bool     `json:"isGitNestrActive"`
	IsAudioActive    bool     `json:"isAudioActive"`
}

func LoadRelaySettings() (*RelaySettings, error) {
	viper.SetConfigName("config") // Name of config file (without extension)
	viper.SetConfigType("json")   // Type of the config file
	viper.AddConfigPath(".")      // Path to look for the config file in

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading config file: %s", err)
		return nil, err
	}

	var settings RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
		log.Fatalf("Error unmarshaling config into struct: %s (nostr/utils)", err)
		return nil, err
	}

	return &settings, nil
}

func IsTheKindAllowed(kind int, settings *RelaySettings) bool {
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

func IsKindBlocked(kind int, settings *RelaySettings) bool {
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
