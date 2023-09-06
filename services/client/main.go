package main

import (
	//"context"
	//"encoding/json"

	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"os"

	keys "github.com/HORNET-Storage/hornet-storage/lib/context"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-multibase"

	"github.com/HORNET-Storage/hornet-storage/lib/encryption/rsa"
)

func main() {
	ctx := context.Background()

	//privateKey := flag.String("private", "", "Private key")
	//publicKey := flag.String("public", "", "Public key")
	//address := flag.String("address", "", "Address")
	flag.Parse()

	//ctx = context.WithValue(ctx, keys.PrivateKey, privateKey)
	//ctx = context.WithValue(ctx, keys.PublicKey, publicKey)
	//ctx = context.WithValue(ctx, keys.Address, address)

	RunCommandWatcher(ctx)
}

func RunCommandWatcher(ctx context.Context) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan

		Cleanup(ctx)
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		scanner.Scan()

		command := strings.TrimSpace(scanner.Text())
		segments := strings.Split(command, " ")

		switch segments[0] {
		case "help":
			log.Println("Available Commands:")
			log.Println("generate")
			log.Println("dag")
			log.Println("shutdown")
		case "generate":
			GenerateKeys(ctx)
		case "parse":
			ParseKeys(ctx)
		case "dag":
			SendTestDag(ctx)
		case "test":
			TestDag(ctx)
		case "shutdown":
			log.Println("Shutting down")
			Cleanup(ctx)
			return
		default:
			log.Printf("Unknown command: %s\n", command)
		}
	}
}

func Cleanup(ctx context.Context) {

}

func GenerateKeys(ctx context.Context) {
	privateKey, err := rsa.CreateKeyPair()
	if err != nil {
		fmt.Println("Failed to create private key")
		return
	}

	rsa.SaveKeyPairToFile(privateKey)
}

func TestDag(ctx context.Context) {
	dag, err := merkle_dag.CreateDag("D:/organizations/akashic_record/unsorted/nostr2.0/testDirectory", multibase.Base64)
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}

	encoder := multibase.MustNewEncoder(multibase.Base64)
	result, err := dag.Verify(encoder)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}

	if result {
		log.Println("Dag verified correctly")
	} else {
		log.Fatal("Dag failed to verify")
	}

	err = dag.CreateDirectory("D:/organizations/akashic_record/unsorted/nostr2.0/newDirectory", encoder)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}
}

type DagLeafMessage struct {
	Root  string
	Count int
	Leaf  merkle_dag.DagLeaf
}

func SendTestDag(ctx context.Context) {
	dag, err := merkle_dag.CreateDag("D:/organizations/akashic_record/unsorted/nostr2.0/testDirectory", multibase.Base64)
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}

	encoder := multibase.MustNewEncoder(multibase.Base64)
	result, err := dag.Verify(encoder)
	if err != nil {
		log.Fatalf("Error: %s", err)
	}

	for _, leaf := range dag.Leafs {

		result, err := leaf.VerifyLeaf(encoder)
		if err != nil {
			log.Fatal(err)
		}

		if result {
			log.Println("Leaf verified correctly")
		} else {
			log.Println("Failed to verify leaf")
		}
	}

	if result {
		log.Println("Dag verified correctly")
	} else {
		log.Fatal("Dag failed to verify")
	}

	client, err := libp2p.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Connect to the server
	serverAddress := "/ip4/0.0.0.0/tcp/9000/p2p/12D3KooWK5w15heWibLQ7KUeKvVwbq8dTaSmad9FxaVD6jtUCT3j" // replace this with the server's multiaddress
	maddr, err := multiaddr.NewMultiaddr(serverAddress)
	if err != nil {
		log.Fatal(err)
	}
	serverInfo, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		log.Fatal(err)
	}
	if err := client.Connect(ctx, *serverInfo); err != nil {
		log.Fatal(err)
	}
	log.Println("Connected to:", serverInfo)

	stream, err := client.NewStream(ctx, serverInfo.ID, "/upload/1.0.0")
	if err != nil {
		log.Fatal(err)
	}

	enc := cbor.NewEncoder(stream)

	for _, leaf := range dag.Leafs {
		message := DagLeafMessage{
			Root:  dag.Root,
			Count: len(dag.Leafs),
			Leaf:  *leaf,
		}

		if err := enc.Encode(&message); err != nil {
			log.Fatal(err)
		}

		time.Sleep(10 * time.Millisecond)
	}

	log.Println("Data sent!")
}

func GetTestDag(ctx context.Context, hash string) {

}

func ParseKeys(ctx context.Context) {
	privateKey, err := rsa.ParsePrivateKeyFromFile("private.key")
	if err != nil {
		fmt.Println("Failed to parse private key")
		return
	}

	publicKey, err := rsa.ParsePublicKeyFromFile("public.pem")
	if err != nil {
		fmt.Println("Failed to parse public key")
		return
	}

	ctx = context.WithValue(ctx, keys.PrivateKey, privateKey)
	ctx = context.WithValue(ctx, keys.PublicKey, publicKey)

	fmt.Println("Keys have been parsed")
}
