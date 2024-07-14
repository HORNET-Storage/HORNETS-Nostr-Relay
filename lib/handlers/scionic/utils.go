package scionic

import (
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

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

func WriteErrorToStream(stream network.Stream, message string, err error) error {
	enc := cbor.NewEncoder(stream)

	if err != nil {
		log.Printf("%s: %v\n", message, err)
	} else {
		log.Println(message)
	}

	data := types.ErrorMessage{
		Message: fmt.Sprintf(message, err),
	}

	if err := enc.Encode(&data); err != nil {
		return err
	}

	return nil
}

func WriteResponseToStream(stream network.Stream, response bool) error {
	streamEncoder := cbor.NewEncoder(stream)

	message := types.ResponseMessage{
		Ok: response,
	}

	if err := streamEncoder.Encode(&message); err != nil {
		return err
	}

	return nil
}

func WaitForResponse(stream network.Stream) bool {
	streamDecoder := cbor.NewDecoder(stream)

	var response types.ResponseMessage

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return false
		default:
			if err := streamDecoder.Decode(&response); err == nil {
				if err == io.EOF {
					return false
				}

				break wait
			}
		}
	}

	return response.Ok
}

func WaitForUploadMessage(stream network.Stream) (bool, *types.UploadMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message types.UploadMessage

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return false, nil
		default:
			err := streamDecoder.Decode(&message)

			if err != nil {
				log.Printf("Error reading from stream: %e", err)
			}

			if err == io.EOF {
				return false, nil
			}

			if err == nil {
				break wait
			}
		}
	}

	return true, &message
}

func WaitForDownloadMessage(stream network.Stream) (bool, *types.DownloadMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message types.DownloadMessage

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return false, nil
		default:
			err := streamDecoder.Decode(&message)

			if err != nil {
				log.Printf("Error reading from stream: %e", err)
			}

			if err == io.EOF {
				return false, nil
			}

			if err == nil {
				break wait
			}
		}
	}

	return true, &message
}

func WaitForQueryMessage(stream network.Stream) (bool, *types.QueryMessage) {
	streamDecoder := cbor.NewDecoder(stream)

	var message types.QueryMessage

	timeout := time.NewTimer(5 * time.Second)

wait:
	for {
		select {
		case <-timeout.C:
			return false, nil
		default:
			err := streamDecoder.Decode(&message)

			if err != nil {
				log.Printf("Error reading from stream: %e", err)
			}

			if err == io.EOF {
				return false, nil
			}

			if err == nil {
				break wait
			}
		}
	}

	return true, &message
}
