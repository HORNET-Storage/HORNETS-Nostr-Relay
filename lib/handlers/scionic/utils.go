package scionic

import (
	"fmt"
	"io"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/spf13/viper"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

type DagWriter func(message interface{}) error

type UploadDagReader func() (*types.UploadMessage, error)
type UploadDagHandler func(read UploadDagReader, write DagWriter)

type DownloadDagReader func() (*types.DownloadMessage, error)
type DownloadDagHandler func(read DownloadDagReader, write DagWriter)

type QueryDagReader func() (*types.QueryMessage, error)
type QueryDagHandler func(read QueryDagReader, write DagWriter)

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

func WaitForAdvancedQueryMessage(stream types.Stream) (*types.AdvancedQueryMessage, error) {
	return ReadMessageFromStream[types.AdvancedQueryMessage](stream)
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

func IsMimeTypePermitted(mimeType string) bool {
	settings, err := RetrieveSettings()
	if err != nil {
		return false
	}

	if len(settings.MimeTypeWhitelist) > 0 {
		if !contains(settings.MimeTypeWhitelist, mimeType) {
			return false
		}
	}

	return true
}

func RetrieveSettings() (*types.RelaySettings, error) {
	var settings types.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

func contains(list []string, item string) bool {
	for _, element := range list {
		if element == item {
			return true
		}
	}
	return false
}
