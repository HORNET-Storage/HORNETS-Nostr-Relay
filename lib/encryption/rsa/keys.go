package rsa

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
)

type PublicKey string
type PrivateKey string

type KeyPair struct {
	PublicKey  PublicKey
	PrivateKey PrivateKey
}

func CreateKeyPair() (*rsa.PrivateKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Printf("Error generating key pair: %v\n", err)
		return nil, err
	}

	return privateKey, nil
}

func SaveKeyPairToFile(privateKey *rsa.PrivateKey) {
	publicKey := &privateKey.PublicKey

	// Save the private key in PEM format.
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	privateKeyFile, err := os.Create("private.key")
	if err != nil {
		fmt.Printf("Error creating private key file: %v\n", err)
		return
	}
	defer privateKeyFile.Close()

	privateKeyFile.Write(privateKeyPEM)

	// Save the public key in PEM format.
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		fmt.Printf("Error marshalling public key: %v\n", err)
		return
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	publicKeyFile, err := os.Create("public.pem")
	if err != nil {
		fmt.Printf("Error creating public key file: %v\n", err)
		return
	}
	defer publicKeyFile.Close()

	publicKeyFile.Write(publicKeyPEM)

	fmt.Println("Private and public keys saved to separate files.")
}

func ParsePrivateKeyFromFile(path string) (*rsa.PrivateKey, error) {
	// Read the private key from the file.
	privateKeyPEM, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("Error reading private key file: %v\n", err)
		return nil, err
	}

	// Decode the PEM-encoded private key.
	privateKeyBlock, _ := pem.Decode(privateKeyPEM)
	if privateKeyBlock == nil {
		fmt.Println("Error decoding private key PEM")
		return nil, fmt.Errorf("Error decoding private key PEM")
	}

	// Parse the private key bytes to create an rsa.PrivateKey object.
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBlock.Bytes)
	if err != nil {
		fmt.Printf("Error parsing private key: %v\n", err)
		return nil, err
	}

	fmt.Println("Successfully read and parsed private key")

	return privateKey, nil
}

func ParsePublicKeyFromFile(path string) (*rsa.PublicKey, error) {
	// Read the public key from the file.
	publicKeyPEM, err := ioutil.ReadFile(path)
	if err != nil {
		fmt.Printf("Error reading public key file: %v\n", err)
		return nil, err
	}

	// Decode the PEM-encoded public key.
	publicKeyBlock, _ := pem.Decode(publicKeyPEM)
	if publicKeyBlock == nil {
		fmt.Println("Error decoding public key PEM")
		return nil, fmt.Errorf("Error decoding public key PEM")
	}

	// Parse the public key bytes to create an rsa.PublicKey object.
	publicKeyInterface, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
	if err != nil {
		fmt.Printf("Error parsing public key: %v\n", err)
		return nil, err
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		fmt.Println("Error asserting publicKeyInterface as *rsa.PublicKey")
		return nil, fmt.Errorf("Error asserting publicKeyInterface as *rsa.PublicKey")
	}

	fmt.Println("Successfully read and parsed public key")

	return publicKey, nil
}
