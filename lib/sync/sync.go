package sync

import (
	"bufio"
	"encoding/hex"
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
const FrameSizeLimit = 4096
const IdSize = 32

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
		err = SendNegentropyMessage(hostId, stream, "NEG-ERR", nostr.Filter{}, []byte{}, err.Error(), []string{}, []*nostr.Event{})
		return
	}

	log.Printf("Successfully completed negentropy sync with %s", remotePeer)
}

func LoadVector(events []*nostr.Event) (*negentropy.Vector, error) {
	vector := negentropy.NewVector()
	for _, event := range events {
		id, err := hex.DecodeString(event.ID)
		if err != nil {
			return nil, err
		}

		err = vector.Insert(uint64(event.CreatedAt), id[:IdSize])
		if err != nil {
			return nil, err
		}
	}

	err := vector.Seal()
	if err != nil {
		return nil, err
	}

	return vector, nil
}

func InitiateNegentropySync(stream network.Stream, filter nostr.Filter, hostId string, store *stores_graviton.GravitonStore) error {
	log.Printf("Performing negentropy on %s", hostId)
	events, err := store.QueryEvents(filter)
	if err != nil {
		return err
	}
	log.Printf("%s has %d events", hostId, len(events))

	// vector conforms to Storage interface, fill it with events
	vector, err := LoadVector(events)
	if err != nil {
		return err
	}

	log.Printf("%s sealed the events", hostId)

	neg, err := negentropy.NewNegentropy(vector, uint64(FrameSizeLimit))
	if err != nil {
		panic(err)
	}

	initialMsg, err := neg.Initiate()
	log.Printf("%s is initiating with version %d", hostId, initialMsg[0])

	err = SendNegentropyMessage(hostId, stream, "NEG-OPEN", filter, initialMsg, "", []string{}, []*nostr.Event{})
	if err != nil {
		return err
	}

	err = listenNegentropy(vector, neg, stream, hostId, store, true)
	return nil
}

func listenNegentropy(vector *negentropy.Vector, neg *negentropy.Negentropy, stream network.Stream, hostId string, store *stores_graviton.GravitonStore, initiator bool) error {
	// Now, start listening to responses and reconcile
	reader := bufio.NewReader(stream)
	final := false

	for {
		response, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break // End of stream
			}
			return fmt.Errorf("error reading from stream: %w", err)
		}
		response = strings.TrimSpace(response)
		//log.Printf("%s received: %s", hostId, response)

		// Create a slice to hold the parsed data
		var parsedData []string

		// Unmarshal the JSON string into the slice
		err = json.Unmarshal([]byte(response), &parsedData)
		if err != nil {
			log.Println("Error parsing JSON:", err)
			return err
		}

		msgType := parsedData[0]
		log.Println(hostId, "Received:", msgType)

		switch msgType {
		case "NEG-OPEN":
			var filter nostr.Filter

			err := json.Unmarshal([]byte(parsedData[2]), &filter)
			if err != nil {
				log.Println("Error unmarshaling filter data:", err)
				return err
			}

			events, err := store.QueryEvents(filter)
			if err != nil {
				return err
			}
			log.Printf("%s has %d events", hostId, len(events))

			vector, err := LoadVector(events)
			if err != nil {
				return err
			}

			log.Printf("%s sealed the events", hostId)
			// intentional shadowing
			neg, err = negentropy.NewNegentropy(vector, uint64(FrameSizeLimit))
			if err != nil {
				return err
			}
			decodedBytes, err := hex.DecodeString(parsedData[4])
			msg, err := neg.Reconcile(decodedBytes)
			if err != nil {
				return err
			}

			err = SendNegentropyMessage(hostId, stream, "NEG-MSG", nostr.Filter{}, msg, "", []string{}, []*nostr.Event{})
			if err != nil {
				return err
			}
			break
		case "NEG-MSG":
			decodedBytes, err := hex.DecodeString(parsedData[2])
			var msg []byte
			var have, need []string

			if initiator {
				//log.Printf("%s neg addr: %x\n", hostId, &neg)
				//log.Printf("%s vector addr: %x\n", hostId, &vector)
				msg, err = neg.ReconcileWithIDs(decodedBytes, &have, &need)
				if err != nil {
					return err
				}
				log.Println(hostId, "have", len(have), "need", len(need))
				//log.Println(hostId, "have:", have, "need:", need)

				// upload have
				if len(have) > 0 {
					hexHave := make([]string, len(have))
					for i, s := range have {
						hexHave[i] = hex.EncodeToString([]byte(s))
					}

					filter := nostr.Filter{
						IDs: hexHave,
					}

					haveEvents, err := store.QueryEvents(filter)
					if err != nil {
						return err
					}
					//log.Println(haveEvents)

					// upload
					err = SendNegentropyMessage(hostId, stream, "NEG-HAVE", nostr.Filter{}, []byte{}, "", []string{}, haveEvents)
					if err != nil {
						return err
					}
				}

				// download need if needed
				if len(need) > 0 {
					needIds := make([]string, len(need))
					for i, s := range need {
						needIds[i] = hex.EncodeToString([]byte(s))
					}
					err = SendNegentropyMessage(hostId, stream, "NEG-NEED", nostr.Filter{}, []byte{}, "", needIds, []*nostr.Event{})
					if err != nil {
						return err
					}
				}
			} else {
				//log.Printf("%s neg addr: %x\n", hostId, &neg)
				//log.Printf("%s vector addr: %x\n", hostId, &vector)
				msg, err = neg.Reconcile(decodedBytes)
			}
			if err != nil {
				return err
			}

			if len(msg) == 0 {
				log.Println(hostId, ": Sync complete")
				if len(need) == 0 {
					// we are done
					err = SendNegentropyMessage(hostId, stream, "NEG-CLOSE", nostr.Filter{}, []byte{}, "", []string{}, []*nostr.Event{})
					if err != nil {
						return err
					}
					return nil
				} else {
					// we are waiting for final needs
					final = true
				}
			} else {
				log.Println(hostId, ": Sync incomplete, drilling down")
				err = SendNegentropyMessage(hostId, stream, "NEG-MSG", nostr.Filter{}, msg, "", []string{}, []*nostr.Event{})
				if err != nil {
					return err
				}
			}
			break

		case "NEG-HAVE":
			var newEvents []*nostr.Event
			err = json.Unmarshal([]byte(parsedData[2]), &newEvents)
			if err != nil {
				return err
			}
			for _, event := range newEvents {
				err := store.StoreEvent(event)
				if err != nil {
					return err
				}
			}
			if final {
				err = SendNegentropyMessage(hostId, stream, "NEG-CLOSE", nostr.Filter{}, []byte{}, "", []string{}, []*nostr.Event{})
				if err != nil {
					return err
				}
				return nil
			}
		case "NEG-NEED":
			var needIds []string

			err = json.Unmarshal([]byte(parsedData[2]), &needIds)
			//log.Printf("other hos t sent %s\n", needIds)

			filter := nostr.Filter{
				IDs: needIds,
			}
			//log.Printf("other host needs %d\n", len(needIds))

			haveEvents, err := store.QueryEvents(filter)
			if err != nil {
				return err
			}

			// upload
			err = SendNegentropyMessage(hostId, stream, "NEG-HAVE", nostr.Filter{}, []byte{}, "", []string{}, haveEvents)
			if err != nil {
				log.Println(hostId, "Error uploading", err)
				return err
			}

		case "NEG-ERR":
			return nil
		case "NEG-CLOSE":
			return nil
		default:
			return errors.New("Unknown message type")
		}
	}
	return nil
}

func SendNegentropyMessage(
	hostId string,
	stream network.Stream,
	msgType string,
	filter nostr.Filter,
	msgBytes []byte,
	errMsg string,
	needIds []string,
	haveEvents []*nostr.Event) error {
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
		msgArray = append(msgArray, string(IdSize))
		msgArray = append(msgArray, msgString)
		break
	case "NEG-MSG":
		msgArray = append(msgArray, msgString)
		break
	case "NEG-ERR":
		msgArray = append(msgArray, errMsg)
		break
	case "NEG-CLOSE":
		break
	case "NEG-HAVE":
		// Marshal the array of events to JSON
		jsonBytes, err := json.Marshal(haveEvents)
		if err != nil {
			log.Println("Error marshaling to JSON:", err)
			return err
		}
		msgArray = append(msgArray, string(jsonBytes))
	case "NEG-NEED":
		jsonBytes, err := json.Marshal(needIds)
		if err != nil {
			log.Println("Error marshaling to JSON:", err)
			return err
		}
		msgArray = append(msgArray, string(jsonBytes))

	default:
		return errors.New("unknown message type")
	}

	jsonData, err := json.Marshal(msgArray)
	if err != nil {
		log.Fatal("Error marshaling JSON:", err)
	}

	//log.Printf("%s sent: %s", hostId, string(jsonData))
	log.Printf("%s sent: %s", hostId, msgType)

	_, err = io.WriteString(stream, string(jsonData)+"\n")
	if err != nil {
		return err
	}

	return nil
}
