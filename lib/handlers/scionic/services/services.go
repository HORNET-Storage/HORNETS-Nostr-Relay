package services

import (
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/spf13/viper"

	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	lib_stream "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr"
	hsListener "github.com/HORNET-Storage/go-hornet-storage-lib/lib/connmgr/hyperswarm"
)

// ServicesResponse is the JSON payload returned by the /services protocol handler.
// It provides the same service discovery information that NIP-11 offers via HTTP,
// enabling clients to discover services purely over the DHT without needing an
// HTTP endpoint. Includes full NIP-11 relay info fields so browser clients can
// display relay metadata without a separate HTTP connection.
type ServicesResponse struct {
	// Relay's secp256k1 public key (Nostr identity)
	Pubkey string `json:"pubkey,omitempty"`

	// Relay's ed25519 DHT public key (the same key used to reach this handler)
	DHTPubkey string `json:"dht_pubkey,omitempty"`

	// Airlock's ed25519 DHT public key for push/upload connections
	AirlockDHTPubkey string `json:"airlock_dht_pubkey,omitempty"`

	// Base port for offset calculations (informational)
	BasePort int `json:"base_port,omitempty"`

	// NIP-11 relay information fields
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	Contact       string `json:"contact,omitempty"`
	Icon          string `json:"icon,omitempty"`
	Software      string `json:"software,omitempty"`
	Version       string `json:"version,omitempty"`
	SupportedNIPs []int  `json:"supported_nips,omitempty"`
}

// AddServicesHandler registers the /services protocol on the hyperswarm listener.
// When a client connects via DHT and opens a /services stream, the handler
// responds with a JSON payload describing the relay's service endpoints.
func AddServicesHandler(listener *hsListener.HyperswarmListener) {
	listener.SetStreamHandler("/services", BuildServicesStreamHandler())
}

// BuildServicesStreamHandler returns the stream handler for /services requests.
func BuildServicesStreamHandler() hsListener.StreamHandler {
	return func(stream lib_types.Stream) {
		defer stream.Close()

		resp := ServicesResponse{
			Pubkey:        viper.GetString("relay.public_key"),
			DHTPubkey:     viper.GetString("DHTPublicKey"),
			BasePort:      viper.GetInt("server.port"),
			Name:          viper.GetString("relay.name"),
			Description:   viper.GetString("relay.description"),
			Contact:       viper.GetString("relay.contact"),
			Icon:          viper.GetString("relay.icon"),
			Software:      viper.GetString("relay.software"),
			Version:       viper.GetString("relay.version"),
			SupportedNIPs: viper.GetIntSlice("relay.supported_nips"),
		}

		// Get airlock DHT pubkey from services config
		airlockDHT := viper.GetString("server.services.airlock.dht_pubkey")
		if airlockDHT != "" {
			resp.AirlockDHTPubkey = airlockDHT
		}

		if err := lib_stream.WriteMessageToStream(stream, resp); err != nil {
			logging.Errorf("/services: failed to write response: %v", err)
			return
		}

		logging.Infof("/services: responded to service discovery request")
	}
}
