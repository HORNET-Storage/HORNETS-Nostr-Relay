package sync

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	stores_graviton "github.com/HORNET-Storage/hornet-storage/lib/stores/graviton"
	"github.com/illuzen/go-negentropy"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/nbd-wtf/go-nostr"
	"io"
	"log"
	"strings"
)

const NegentropyProtocol = "/negentropy/1.0.0"
const frameSizeLimit = 4096

func split(s string, delim rune) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == delim
	})
}

func SetupNegentropyHandler(h host.Host, hostId string, db *stores_graviton.GravitonStore) {
	handler := func(stream network.Stream) {
		handleIncomingNegentropyStream(stream, hostId, db)
	}
	h.SetStreamHandler(NegentropyProtocol, handler)
}

func handleIncomingNegentropyStream(stream network.Stream, hostId string, store *stores_graviton.GravitonStore) {
	defer stream.Close()

	// Log the incoming connection (optional)
	localPeer := stream.Conn().LocalPeer()
	remotePeer := stream.Conn().RemotePeer()
	log.Printf("Received negentropy sync request to %s from %s", localPeer, remotePeer)

	// Perform the negentropy sync
	vector := negentropy.NewVector()
	err := listenNegentropy(vector, &negentropy.Negentropy{}, stream, hostId, store, false)
	if err != nil {
		messageArray := []interface{}{
			"NEG-ERR",
			"N",
			err.Error(),
		}
		jsonData, _ := json.Marshal(messageArray)
		_, _ = io.WriteString(stream, string(jsonData)+"\n")

		return
	}

	log.Printf("Successfully completed negentropy sync with %s", remotePeer)
}

func InitiateNegentropySync(stream network.Stream, filter nostr.Filter, hostId string, store *stores_graviton.GravitonStore) error {
	log.Printf("Performing negentropy on %s", hostId)
	events, err := store.QueryEvents(filter)
	if err != nil {
		return err
	}
	log.Printf("%s has %d events", hostId, len(events))

	// vector conforms to Storage interface, fill it with events
	vector := negentropy.NewVector()
	for _, event := range events {
		err = vector.Insert(uint64(event.CreatedAt), event.Serialize()[:32])
		if err != nil {
			return err
		}
	}

	err = vector.Seal()
	if err != nil {
		return err
	}
	log.Printf("%s sealed the events", hostId)

	idSize := 16

	neg, err := negentropy.NewNegentropy(vector, uint64(frameSizeLimit))
	if err != nil {
		panic(err)
	}

	initialMsg, err := neg.Initiate()
	log.Printf("%s is initiating with version %d", hostId, uint8(initialMsg[0]))

	messageArray := []interface{}{
		"NEG-OPEN",
		"N",
		filter,
		idSize,
		initialMsg,
	}
	jsonData, err := json.Marshal(messageArray)
	if err != nil {
		log.Fatal("Error marshaling JSON:", err)
	}

	log.Printf("%s sent: %s", hostId, jsonData)

	_, err = io.WriteString(stream, string(jsonData)+"\n")
	if err != nil {
		return fmt.Errorf("failed to send initial query: %w", err)
	}

	err = listenNegentropy(vector, neg, stream, hostId, store, true)
	return nil
}

func listenNegentropy(vector *negentropy.Vector, neg *negentropy.Negentropy, stream network.Stream, hostId string, store *stores_graviton.GravitonStore, initiator bool) error {
	// Now, start listening to responses and reconcile
	reader := bufio.NewReader(stream)

	for {
		response, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break // End of stream
			}
			return fmt.Errorf("error reading from stream: %w", err)
		}
		response = strings.TrimSpace(response)
		log.Printf("%s received: %s", hostId, response)

		// Create a slice to hold the parsed data
		var parsedData []interface{}

		// Unmarshal the JSON string into the slice
		err = json.Unmarshal([]byte(response), &parsedData)
		if err != nil {
			fmt.Println("Error parsing JSON:", err)
			return err
		}

		msgType := parsedData[0].(string)

		switch msgType {
		case "NEG-OPEN":
			fmt.Println(hostId, "Received:", parsedData)

			var filter nostr.Filter
			filterJSON, err := json.Marshal(parsedData[2])
			if err != nil {
				fmt.Println("Error re-marshaling filter data:", err)
				return err
			}

			err = json.Unmarshal(filterJSON, &filter)
			if err != nil {
				fmt.Println("Error parsing filter:", err)
				return err
			}

			events, err := store.QueryEvents(filter)
			if err != nil {
				return err
			}
			log.Printf("%s has %d events", hostId, len(events))

			for _, event := range events {
				err = vector.Insert(uint64(event.CreatedAt), event.Serialize()[:32])
				if err != nil {
					return err
				}
			}

			err = vector.Seal()
			if err != nil {
				return err
			}
			log.Printf("%s sealed the events", hostId)
			// intentional shadowing
			neg, err := negentropy.NewNegentropy(vector, uint64(frameSizeLimit))
			if err != nil {
				return err
			}
			decodedBytes, err := base64.StdEncoding.DecodeString(parsedData[4].(string))
			msg, err := neg.Reconcile(decodedBytes)
			if err != nil {
				return err
			}

			messageArray := []interface{}{
				"NEG-MSG",
				"N",
				msg,
			}
			jsonData, err := json.Marshal(messageArray)
			if err != nil {
				log.Fatal("Error marshaling JSON:", err)
			}

			log.Printf("%s sent: %s", hostId, string(jsonData))

			_, err = io.WriteString(stream, string(jsonData)+"\n")
			if err != nil {
				return err
			}

			break
		case "NEG-MSG":
			fmt.Println(hostId, "Received:", msgType)
			decodedBytes, err := base64.StdEncoding.DecodeString(parsedData[2].(string))
			var msg []byte
			if initiator {
				var have, need []string
				msg, err = neg.ReconcileWithIDs(decodedBytes, &have, &need)
				fmt.Println(hostId, "have:", have, "need:", need)
				// TODO: upload have
				// TODO: download need
			} else {
				msg, err = neg.Reconcile(decodedBytes)
			}
			if err != nil {
				return err
			}

			if len(msg) == 0 {
				fmt.Println(hostId, ": Sync complete")
				return nil
			}
			messageArray := []interface{}{
				"NEG-MSG",
				"N",
				msg,
			}
			jsonData, err := json.Marshal(messageArray)
			if err != nil {
				log.Fatal("Error marshaling JSON:", err)
			}

			log.Printf("%s sent: %s", hostId, string(jsonData))

			_, err = io.WriteString(stream, string(jsonData)+"\n")
			if err != nil {
				return err
			}

			break
		case "NEG-ERR":
			fmt.Println(hostId, "Received:", msgType)
			return nil
		case "NEG-CLOSE":
			fmt.Println(hostId, "Received:", msgType)
			return nil
		default:
			fmt.Println(hostId, "Received:", msgType)
			return errors.New("Unknown message type")
		}
	}
	return nil
}

func DeserializeEvent(data []byte) (*nostr.Event, error) {
	var arr []json.RawMessage
	err := json.Unmarshal(data, &arr)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal event array: %w", err)
	}

	if len(arr) != 6 {
		return nil, fmt.Errorf("invalid event array length: expected 6, got %d", len(arr))
	}

	var pubkey string
	err = json.Unmarshal(arr[1], &pubkey)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal pubkey: %w", err)
	}

	var createdAt int64
	err = json.Unmarshal(arr[2], &createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal created_at: %w", err)
	}

	var kind int
	err = json.Unmarshal(arr[3], &kind)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal kind: %w", err)
	}

	var tags nostr.Tags
	err = json.Unmarshal(arr[4], &tags)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	var content string
	err = json.Unmarshal(arr[5], &content)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	event := &nostr.Event{
		PubKey:    pubkey,
		CreatedAt: nostr.Timestamp(createdAt),
		Kind:      kind,
		Tags:      tags,
		Content:   content,
	}

	// Calculate the ID
	event.ID = event.GetID()

	return event, nil
}
