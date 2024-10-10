package scionic

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/spf13/viper"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

type DagWriter func(message interface{}) error

type UploadDagReader func() (*types.UploadMessage, error)
type UploadDagHandler func(read UploadDagReader, write DagWriter)

type DownloadDagReader func() (*types.DownloadMessage, error)
type DownloadDagHandler func(read DownloadDagReader, write DagWriter)

type QueryDagReader func() (*types.QueryMessage, error)
type QueryDagHandler func(read QueryDagReader, write DagWriter)

func CheckFilter(leaf *merkle_dag.DagLeaf, filter *types.DownloadFilter) (bool, error) {
	label := merkle_dag.GetLabel(leaf.Hash)

	if len(filter.Leaves) <= 0 && len(filter.LeafRanges) <= 0 {
		return true, nil
	}

	if slices.Contains(filter.Leaves, label) {
		return true, nil
	}

	labelInt, err := strconv.Atoi(label)
	if err != nil {
		return false, err
	}

	for _, rangeItem := range filter.LeafRanges {
		fromInt, err := strconv.Atoi(rangeItem.From)
		if err != nil {
			continue // Skip invalid ranges
		}
		toInt, err := strconv.Atoi(rangeItem.To)
		if err != nil {
			continue // Skip invalid ranges
		}

		if labelInt >= fromInt && labelInt <= toInt {
			return true, nil
		}
	}

	return false, nil
}

func BuildErrorMessage(message string, err error) types.ErrorMessage {
	return types.ErrorMessage{
		Message: fmt.Sprintf(message, err),
	}
}

func BuildResponseMessage(response bool) types.ResponseMessage {
	return types.ResponseMessage{
		Ok: response,
	}
}

func WriteErrorToStream(stream types.Stream, message string, err error) error {
	return WriteMessageToStream(stream, BuildErrorMessage(message, err))
}

func WriteResponseToStream(stream types.Stream, response bool) error {
	return WriteMessageToStream(stream, BuildResponseMessage(response))
}

func WaitForResponse(stream types.Stream) (*types.ResponseMessage, error) {
	return ReadMessageFromStream[types.ResponseMessage](stream)
}

func WaitForUploadMessage(stream types.Stream) (*types.UploadMessage, error) {
	return ReadMessageFromStream[types.UploadMessage](stream)
}

func WaitForDownloadMessage(stream types.Stream) (*types.DownloadMessage, error) {
	return ReadMessageFromStream[types.DownloadMessage](stream)
}

func WaitForQueryMessage(stream types.Stream) (*types.QueryMessage, error) {
	return ReadMessageFromStream[types.QueryMessage](stream)
}

func ReadMessageFromStream[T any](stream types.Stream) (*T, error) {
	streamDecoder := cbor.NewDecoder(stream)

	var message T

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return nil, fmt.Errorf("WaitForMessage timed out")
		default:
			err := streamDecoder.Decode(&message)

			if err != nil {
				return nil, err
			}

			if err == io.EOF {
				return nil, err
			}

			break wait
		}
	}

	return &message, nil
}

func WriteMessageToStream[T any](stream types.Stream, message T) error {
	enc := cbor.NewEncoder(stream)

	if err := enc.Encode(&message); err != nil {
		return err
	}

	return nil
}

// Function to check file permission based on RelaySettings, loading settings internally
func IsFilePermitted(filename string) bool {
	// Load relay settings
	settings, err := LoadRelaySettings()
	if err != nil {
		log.Fatalf("Failed to load relay settings: %v", err)
		return false
	}

	// Extract the file extension and make it lowercase for case-insensitive comparison
	fileExtension := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))

	// Check mode setting
	if settings.Mode == "smart" {
		// Smart mode: Check if the file extension is explicitly allowed

		if settings.IsPhotosActive && contains(settings.PhotoTypes, fileExtension) {
			return true
		}
		if settings.IsVideosActive && contains(settings.VideoTypes, fileExtension) {
			return true
		}
		if settings.IsAudioActive && contains(settings.AudioTypes, fileExtension) {
			return true
		}

		// Miscellaneous case: If the file type is not in the known lists but also not blocked, allow it
		if !contains(settings.Photos, fileExtension) &&
			!contains(settings.Videos, fileExtension) &&
			!contains(settings.Audio, fileExtension) {
			return true // Permit miscellaneous files in smart mode if they are not explicitly blocked
		}

		return false // File type is not permitted in "smart" mode if it doesn't match any active or miscellaneous type
	} else if settings.Mode == "unlimited" {
		// Unlimited mode: Allow everything except explicitly blocked types

		if contains(settings.Photos, fileExtension) || contains(settings.Videos, fileExtension) || contains(settings.Audio, fileExtension) {
			return false // File type is explicitly blocked in "unlimited" mode
		}

		return true // File type is permitted in "unlimited" mode
	}

	return false // Default to false if the mode is not recognized
}

// Helper function to check if a slice contains a given string
func contains(list []string, item string) bool {
	for _, element := range list {
		if element == item {
			return true
		}
	}
	return false
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
