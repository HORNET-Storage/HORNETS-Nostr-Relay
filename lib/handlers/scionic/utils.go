package scionic

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/fxamacker/cbor/v2"

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
