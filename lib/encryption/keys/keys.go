package keys

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	//"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func CreateKey() {
	// Generate a private key using btcec.
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		log.Fatal("Failed to generate private key:", err)
	}

	// Derive the public key.
	pubKey := privKey.PubKey()

	// Serialize the public key in compressed format.
	pubKeySerialized := pubKey.SerializeCompressed()

	// Create a Bitcoin mainnet P2PKH address.
	//addr, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pubKeySerialized), &chaincfg.MainNetParams)
	//if err != nil {
	//	log.Fatal("Failed to create address:", err)
	//}

	wif, err := btcutil.NewWIF(privKey, &chaincfg.Params{}, true)
	if err != nil {
		log.Fatal("Failed to create wif:", err)
	}

	fmt.Println("Private Key (WIF):", wif)
	fmt.Println("Public Key (Compressed):", fmt.Sprintf("%x", pubKeySerialized))
	//fmt.Println("Bitcoin Address:", addr)

	SaveHexFile("./public-key.hex", pubKeySerialized)
	SaveHexFile("./private-key.hex", privKey.Serialize())
}

func SaveHexFile(filename string, data []byte) error {
	hexData := hex.EncodeToString(data)
	return ioutil.WriteFile(filename, []byte(hexData), 0600)
}

func LoadHexFile(filename string) ([]byte, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(string(data))
}
