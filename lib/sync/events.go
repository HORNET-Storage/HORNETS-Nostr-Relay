package sync

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/illuzen/go-negentropy"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/nbd-wtf/go-nostr"
)

// TODO: where is this supposed to come from? config file?
const hornetNpub string = "npub1c25aedfd38f9fed72b383f6eefaea9f21dd58ec2c9989e0cc275cb5296adec17"

func SetupNegentropyEventHandler(h host.Host, hostId string, store stores.Store) {
	handler := func(stream network.Stream) {
		handleIncomingNegentropyEventStream(stream, hostId, store)
	}
	h.SetStreamHandler(NegentropyProtocol, handler)
}

func handleIncomingNegentropyEventStream(stream network.Stream, hostId string, store stores.Store) {
	defer stream.Close()

	// Log the incoming connection (optional)
	localPeer := stream.Conn().LocalPeer()
	remotePeer := stream.Conn().RemotePeer()
	logging.Infof("Received negentropy sync request to %s from %s", localPeer, remotePeer)

	// Perform the negentropy sync
	err := listenNegentropy(&negentropy.Negentropy{}, stream, hostId, store, false)
	if err != nil {
		// Send error message but don't overwrite the original error
		if sendErr := SendNegentropyMessage(hostId, stream, "NEG-ERR", nostr.Filter{}, []byte{}, err.Error(), []string{}, []byte{}); sendErr != nil {
			logging.Infof("Failed to send error message: %v", sendErr)
		}
		return
	}

	logging.Infof("Successfully completed negentropy sync with %s", remotePeer)
}

func LoadEventVector(events []*nostr.Event) (*negentropy.Vector, error) {
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

func InitiateEventSync(stream network.Stream, filter nostr.Filter, hostId string, store stores.Store) error {
	logging.Infof("Performing negentropy on %s", hostId)
	events, err := store.QueryEvents(filter)
	if err != nil {
		return err
	}
	logging.Infof("%s has %d events", hostId, len(events))

	// vector conforms to Storage interface, fill it with events
	vector, err := LoadEventVector(events)
	if err != nil {
		return err
	}

	logging.Infof("%s sealed the events", hostId)

	neg, err := negentropy.NewNegentropy(vector, uint64(FrameSizeLimit))
	if err != nil {
		panic(err)
	}

	initialMsg, err := neg.Initiate()
	if err != nil {
		return fmt.Errorf("failed to initiate negentropy: %w", err)
	}
	logging.Infof("%s is initiating with version %d", hostId, initialMsg[0])

	if err = SendNegentropyMessage(hostId, stream, "NEG-OPEN", filter, initialMsg, "", []string{}, []byte{}); err != nil {
		return err
	}

	return listenNegentropy(neg, stream, hostId, store, true)
}

func GetScionicRoot(event *nostr.Event) (string, bool) {
	for _, tag := range event.Tags {
		if len(tag) == 2 && tag[0] == "scionic_root" {
			return tag[1], true
		}
	}
	return "", false
}

func DownloadDag(root string) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Connect to a hornet storage node
	publicKey, err := signing.DeserializePublicKey(hornetNpub)
	if err != nil {
		logging.Fatalf("Failed to deserialize public key: %v", err)
	}

	libp2pPubKey, err := signing.ConvertPubKeyToLibp2pPubKey(publicKey)
	if err != nil {
		logging.Fatalf("Failed to convert pubkey to libp2p pubkey: %v", err)
	}

	peerId, err := peer.IDFromPublicKey(*libp2pPubKey)
	if err != nil {
		logging.Fatalf("Failed to get peer ID from public key: %v", err)
	}

	conMgr := connmgr.NewGenericConnectionManager()

	err = conMgr.ConnectWithLibp2p(ctx, "default", fmt.Sprintf("/ip4/127.0.0.1/udp/9000/quic-v1/p2p/%s", peerId.String()), libp2p.Transport(libp2pquic.NewTransport))
	if err != nil {
		logging.Fatalf("Failed to connect with libp2p: %v", err)
	}

	progressChan := make(chan lib.DownloadProgress)

	go func() {
		for progress := range progressChan {
			if progress.Error != nil {
				logging.Infof("Error uploading to %s: %v\n", progress.ConnectionID, progress.Error)
			} else {
				logging.Infof("Progress for %s: %d leafs downloaded\n", progress.ConnectionID, progress.LeafsRetreived)
			}
		}
	}()

	// Upload the dag to the hornet storage node
	_, dag, err := connmgr.DownloadDag(ctx, conMgr, "default", root, nil, nil, progressChan)
	if err != nil {
		logging.Fatalf("Failed to download DAG: %v", err)
	}

	close(progressChan)

	// Verify the entire dag
	err = dag.Dag.Verify()
	if err != nil {
		logging.Fatalf("Error: %s", err)
	}

	logging.Info("Dag verified correctly")

	// Disconnect client as we no longer need it
	err = conMgr.Disconnect("default")
	if err != nil {
		logging.Infof("Could not disconnect from hornet storage: %v", err)
	}
}

func listenNegentropy(neg *negentropy.Negentropy, stream network.Stream, hostId string, store stores.Store, initiator bool) error {
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
		//logging.Infof("%s received: %s", hostId, response)

		// Create a slice to hold the parsed data
		var parsedData []string

		// Unmarshal the JSON string into the slice
		err = json.Unmarshal([]byte(response), &parsedData)
		if err != nil {
			logging.Infof("Error parsing JSON:%s", err)
			return err
		}

		msgType := parsedData[0]
		logging.Infof(hostId, "Received:%s", msgType)

		switch msgType {
		case "NEG-OPEN":
			var filter nostr.Filter

			err := json.Unmarshal([]byte(parsedData[2]), &filter)
			if err != nil {
				logging.Infof("Error unmarshaling filter data:%s", err)
				return err
			}

			events, err := store.QueryEvents(filter)
			if err != nil {
				return err
			}
			logging.Infof("%s has %d events", hostId, len(events))

			vector, err := LoadEventVector(events)
			if err != nil {
				return err
			}

			logging.Infof("%s sealed the events", hostId)
			// intentional shadowing
			neg, err = negentropy.NewNegentropy(vector, uint64(FrameSizeLimit))
			if err != nil {
				return err
			}
			decodedBytes, err := hex.DecodeString(parsedData[4])
			if err != nil {
				return fmt.Errorf("failed to decode hex string: %w", err)
			}
			msg, err := neg.Reconcile(decodedBytes)
			if err != nil {
				return err
			}

			if err = SendNegentropyMessage(hostId, stream, "NEG-MSG", nostr.Filter{}, msg, "", []string{}, []byte{}); err != nil {
				return err
			}
		case "NEG-MSG":
			decodedBytes, err := hex.DecodeString(parsedData[2])
			if err != nil {
				return fmt.Errorf("failed to decode hex string: %w", err)
			}
			var msg []byte
			var have, need []string

			if initiator {
				msg, err = neg.ReconcileWithIDs(decodedBytes, &have, &need)
				if err != nil {
					return err
				}
				logging.Infof(hostId, "have", len(have), "need", len(need))
				//logging.Info(hostId, "have:", have, "need:", need)

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
					//logging.Info(haveEvents)

					// Marshal the array of events to JSON
					haveBytes, err := json.Marshal(haveEvents)
					if err != nil {
						logging.Infof("Error marshaling to JSON:%s", err)
						return err
					}

					// upload
					if err = SendNegentropyMessage(hostId, stream, "NEG-HAVE", nostr.Filter{}, []byte{}, "", []string{}, haveBytes); err != nil {
						return err
					}
				}

				// download need if needed
				if len(need) > 0 {
					needIds := make([]string, len(need))
					for i, s := range need {
						needIds[i] = hex.EncodeToString([]byte(s))
					}
					if err = SendNegentropyMessage(hostId, stream, "NEG-NEED", nostr.Filter{}, []byte{}, "", needIds, []byte{}); err != nil {
						return err
					}
				}
			} else {
				msg, err = neg.Reconcile(decodedBytes)
			}
			if err != nil {
				return err
			}

			if len(msg) == 0 {
				logging.Infof(hostId, "%s: Sync complete")
				if len(need) == 0 {
					// we are done
					if err = SendNegentropyMessage(hostId, stream, "NEG-CLOSE", nostr.Filter{}, []byte{}, "", []string{}, []byte{}); err != nil {
						return err
					}
					return nil
				} else {
					// we are waiting for final needs
					final = true
				}
			} else {
				logging.Infof(hostId, "%s: Sync incomplete, drilling down")
				if err = SendNegentropyMessage(hostId, stream, "NEG-MSG", nostr.Filter{}, msg, "", []string{}, []byte{}); err != nil {
					return err
				}
			}

		case "NEG-HAVE":
			var newEvents []*nostr.Event
			err = json.Unmarshal([]byte(parsedData[2]), &newEvents)
			if err != nil {
				return err
			}
			for _, event := range newEvents {
				err := store.StoreEvent(event)
				if err != nil {
					logging.Infof("Could not store event %+v skipping", event)
					continue
				}
				// TODO: this needs to be more thoroughly tested
				if event.Kind == 117 {
					// do leaf sync
					root, found := GetScionicRoot(event)
					if !found {
						logging.Infof("Event of type 117 with no 'scionic_root' tag, skipping tree download %+v", event)
						continue
					}

					DownloadDag(root)
				}
			}
			if final {
				if err = SendNegentropyMessage(hostId, stream, "NEG-CLOSE", nostr.Filter{}, []byte{}, "", []string{}, []byte{}); err != nil {
					return err
				}
				return nil
			}
		case "NEG-NEED":
			var needIds []string

			err = json.Unmarshal([]byte(parsedData[2]), &needIds)
			if err != nil {
				return fmt.Errorf("failed to unmarshal need IDs: %w", err)
			}
			//logging.Infof("other hos t sent %s\n", needIds)

			filter := nostr.Filter{
				IDs: needIds,
			}
			//logging.Infof("other host needs %d\n", len(needIds))

			haveEvents, err := store.QueryEvents(filter)
			if err != nil {
				return err
			}

			// Marshal the array of events to JSON
			haveBytes, err := json.Marshal(haveEvents)
			if err != nil {
				logging.Infof("Error marshaling to JSON:%s", err)
				return err
			}

			// upload
			if err = SendNegentropyMessage(hostId, stream, "NEG-HAVE", nostr.Filter{}, []byte{}, "", []string{}, haveBytes); err != nil {
				logging.Infof(hostId, "Error uploading", err)
				return err
			}

		case "NEG-ERR":
			return nil
		case "NEG-CLOSE":
			return nil
		default:
			return errors.New("unknown message type")
		}
	}
	return nil
}
