package sync

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/nbd-wtf/go-nostr"
)

const NegentropyProtocol = "/negentropy/1.0.0"
const FrameSizeLimit = 4096
const IdSize = 32

func SendNegentropyMessage(
	hostId string,
	stream network.Stream,
	msgType string,
	filter nostr.Filter,
	msgBytes []byte,
	errMsg string,
	needIds []string,
	haveBytes []byte) error {
	var msgArray []string
	msgArray = append(msgArray, msgType)
	msgArray = append(msgArray, "N")
	msgString := hex.EncodeToString(msgBytes)
	switch msgType {
	case "NEG-OPEN":
		jsonFilter, err := json.Marshal(filter)
		if err != nil {
			return err
		}
		msgArray = append(msgArray, string(jsonFilter))
		msgArray = append(msgArray, strconv.Itoa(IdSize))
		msgArray = append(msgArray, msgString)
	case "NEG-MSG":
		msgArray = append(msgArray, msgString)
	case "NEG-ERR":
		msgArray = append(msgArray, errMsg)
	case "NEG-CLOSE":
	case "NEG-HAVE":

		msgArray = append(msgArray, string(haveBytes))
	case "NEG-NEED":
		jsonBytes, err := json.Marshal(needIds)
		if err != nil {
			logging.Infof("Error marshaling to JSON:%s", err)
			return err
		}
		msgArray = append(msgArray, string(jsonBytes))

	default:
		return errors.New("unknown message type")
	}

	jsonData, err := json.Marshal(msgArray)
	if err != nil {
		logging.Fatalf("Error marshaling JSON: %v", err)
	}

	//logging.Infof("%s sent: %s", hostId, string(jsonData))
	logging.Infof("%s sent: %s", hostId, msgType)

	_, err = io.WriteString(stream, string(jsonData)+"\n")
	if err != nil {
		return err
	}

	return nil
}
