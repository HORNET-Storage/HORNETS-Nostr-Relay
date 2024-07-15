package sync

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
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

func split(s string, delim rune) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == delim
	})
}

func SetupNegentropyHandler(h host.Host, db *stores_graviton.GravitonStore) {
	handler := func(stream network.Stream) {
		handleIncomingNegentropyStream(stream, h, db)
	}
	h.SetStreamHandler(NegentropyProtocol, handler)
}

func handleIncomingNegentropyStream(stream network.Stream, h host.Host, db *stores_graviton.GravitonStore) {
	defer stream.Close()

	// Log the incoming connection (optional)
	localPeer := stream.Conn().LocalPeer()
	remotePeer := stream.Conn().RemotePeer()
	log.Printf("Received negentropy sync request to %s from %s", localPeer, remotePeer)

	// Perform the negentropy sync
	err := PerformNegentropySync(stream, h, db, false)
	if err != nil {
		log.Printf("Error during negentropy sync with %s: %v", remotePeer, err)
		// Optionally, you might want to send an error message to the peer
		// stream.Write([]byte("Sync failed"))
		return
	}

	log.Printf("Successfully completed negentropy sync with %s", remotePeer)
}

func PerformNegentropySync(stream network.Stream, h host.Host, store *stores_graviton.GravitonStore, initiator bool) error {
	log.Printf("Performing negentropy on %s", h.ID())
	filter := nostr.Filter{}
	events, err := store.QueryEvents(filter)
	if err != nil {
		return err
	}
	log.Printf("%s has %d events", h.ID(), len(events))

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
	log.Printf("%s sealed the events", h.ID())

	frameSizeLimit := 4096
	neg, err := negentropy.NewNegentropy(vector, uint64(frameSizeLimit))
	if err != nil {
		panic(err)
	}

	if initiator {
		initialMsg, err := neg.Initiate()
		log.Printf("%s is initiating with version %s", h.ID(), initialMsg[0])

		message := fmt.Sprintf("msg,%x\n", initialMsg)
		log.Printf("%s sent: %s", h.ID(), message)

		_, err = io.WriteString(stream, message)
		if err != nil {
			return fmt.Errorf("failed to send initial query: %w", err)
		}
	}

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
		log.Printf("%s received: %s", h.ID(), response)

		items := split(response, ',')

		switch items[0] {
		case "msg":
			var q []byte
			if len(items) >= 2 {
				s, err := hex.DecodeString(items[1])
				if err != nil {
					panic(err)
				}
				q = s
			}

			if initiator {
				var have, need []string
				resp, err := neg.ReconcileWithIDs(q, &have, &need)
				if err != nil {
					panic(fmt.Sprintf("Reconciliation failed: %v", err))
				}

				for _, id := range have {
					message := fmt.Sprintf("have,%s\n", hex.EncodeToString([]byte(id)))
					_, err = io.WriteString(stream, message)
					if err != nil {
						return fmt.Errorf("failed to send initial query: %w", err)
					}
					fmt.Printf("have,%s\n", hex.EncodeToString([]byte(id)))

				}
				for _, id := range need {
					message := fmt.Sprintf("need,%s\n", hex.EncodeToString([]byte(id)))
					_, err = io.WriteString(stream, message)
					if err != nil {
						return fmt.Errorf("failed to send initial query: %w", err)
					}

					fmt.Printf("need,%s\n", hex.EncodeToString([]byte(id)))
				}

				if resp == nil {
					message := fmt.Sprintf("done")
					_, err = io.WriteString(stream, message)
					if err != nil {
						return fmt.Errorf("failed to send initial query: %w", err)
					}
					fmt.Println("done")
					continue
				}

				q = resp
			} else {
				s, err := neg.Reconcile(q)
				if err != nil {
					panic(fmt.Sprintf("Reconciliation failed: %v", err))
				}
				q = s
			}

			if frameSizeLimit > 0 && len(q) > frameSizeLimit {
				panic("frameSizeLimit exceeded")
			}

			message := fmt.Sprintf("msg,%s\n", hex.EncodeToString(q))
			_, err = io.WriteString(stream, message)
			if err != nil {
				return fmt.Errorf("failed to send initial query: %w", err)
			}
		case "done":
			// put the vector into graviton
			// this would be more efficient to just make graviton conform to Storage interface...

			for i := 1; i <= vector.Size(); i++ {
				eventBytes, err := vector.GetItem(uint64(i))
				if err != nil {
					return err
				}
				event, err := DeserializeEvent(eventBytes.ID)
				err = store.StoreEvent(event)
				if err != nil {
					return err
				}
			}
		default:
			panic("unknown cmd: " + items[0])

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
