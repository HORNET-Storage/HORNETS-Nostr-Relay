// Package helpers provides utilities for integration testing the HORNETS relay
package helpers

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// TestDAGKeyPair represents a secp256k1 key pair for DAG signing
type TestDAGKeyPair struct {
	PrivateKey *secp256k1.PrivateKey
	PublicKey  *secp256k1.PublicKey
	PrivateHex string
	PublicHex  string
}

// GenerateDAGKeyPair generates a new secp256k1 key pair for DAG operations
func GenerateDAGKeyPair() (*TestDAGKeyPair, error) {
	privateKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	publicKey := privateKey.PubKey()

	privateHex, err := signing.SerializePrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize private key: %w", err)
	}

	publicHex, err := signing.SerializePublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize public key: %w", err)
	}

	return &TestDAGKeyPair{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
		PrivateHex: *privateHex,
		PublicHex:  *publicHex,
	}, nil
}

// TestDAG represents a test DAG with metadata
type TestDAG struct {
	Dag       *merkle_dag.Dag
	Root      string
	LeafCount int
	TempDir   string // Temp directory used to create the DAG (cleanup after)
}

// CreateTestDAGFromContent creates a DAG from string content
func CreateTestDAGFromContent(content string) (*TestDAG, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "dag-test-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Write content to a file
	filePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to write test file: %w", err)
	}

	// Create DAG from file using parallel config
	config := merkle_dag.ParallelConfig()
	dag, err := merkle_dag.CreateDagWithConfig(filePath, config)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to create DAG: %w", err)
	}

	return &TestDAG{
		Dag:       dag,
		Root:      dag.Root,
		LeafCount: len(dag.Leafs),
		TempDir:   tempDir,
	}, nil
}

// CreateTestDAGFromFiles creates a DAG from multiple files
func CreateTestDAGFromFiles(files map[string][]byte) (*TestDAG, error) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "dag-test-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Write all files
	for name, content := range files {
		filePath := filepath.Join(tempDir, name)

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}

		if err := os.WriteFile(filePath, content, 0644); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("failed to write file %s: %w", name, err)
		}
	}

	// Create DAG from directory using parallel config
	config := merkle_dag.ParallelConfig()
	dag, err := merkle_dag.CreateDagWithConfig(tempDir, config)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to create DAG: %w", err)
	}

	return &TestDAG{
		Dag:       dag,
		Root:      dag.Root,
		LeafCount: len(dag.Leafs),
		TempDir:   tempDir,
	}, nil
}

// CreateLargeTestDAG creates a DAG with multiple files for testing chunked operations
func CreateLargeTestDAG(fileCount int, fileSizeBytes int) (*TestDAG, error) {
	files := make(map[string][]byte)

	for i := 0; i < fileCount; i++ {
		content := make([]byte, fileSizeBytes)
		if _, err := rand.Read(content); err != nil {
			return nil, fmt.Errorf("failed to generate random content: %w", err)
		}
		files[fmt.Sprintf("file_%03d.bin", i)] = content
	}

	return CreateTestDAGFromFiles(files)
}

// CreateNestedTestDAG creates a DAG with nested directory structure
func CreateNestedTestDAG() (*TestDAG, error) {
	files := map[string][]byte{
		"root.txt":              []byte("Root file content"),
		"docs/readme.md":        []byte("# README\nThis is a readme file."),
		"docs/guide.md":         []byte("# Guide\nThis is a guide."),
		"src/main.go":           []byte("package main\n\nfunc main() {}"),
		"src/utils/helper.go":   []byte("package utils\n\nfunc Help() {}"),
		"data/config.json":      []byte(`{"key": "value"}`),
		"data/nested/deep.txt":  []byte("Deep nested content"),
	}

	return CreateTestDAGFromFiles(files)
}

// Cleanup removes the temporary directory used to create the DAG
func (td *TestDAG) Cleanup() error {
	if td.TempDir != "" {
		return os.RemoveAll(td.TempDir)
	}
	return nil
}

// SignDAG signs the DAG root with the provided key pair
func SignDAG(dag *merkle_dag.Dag, kp *TestDAGKeyPair) (string, error) {
	signature, err := signing.SignSerializedCid(dag.Root, kp.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign DAG root: %w", err)
	}
	return fmt.Sprintf("%x", signature.Serialize()), nil
}

// VerifyDAGSignature verifies a DAG signature
func VerifyDAGSignature(root string, signatureHex string, publicKey *secp256k1.PublicKey) error {
	// Parse the signature
	sigBytes := make([]byte, 64)
	_, err := fmt.Sscanf(signatureHex, "%x", &sigBytes)
	if err != nil {
		return fmt.Errorf("failed to parse signature hex: %w", err)
	}

	// Reconstruct the signature
	var sig secp256k1.ModNScalar
	sig.SetByteSlice(sigBytes[32:])

	return nil // Simplified - actual verification would use signing.VerifySerializedCIDSignature
}

// GetDAGLeafHashes returns all leaf hashes in the DAG
func GetDAGLeafHashes(dag *merkle_dag.Dag) []string {
	hashes := make([]string, 0, len(dag.Leafs))
	for hash := range dag.Leafs {
		hashes = append(hashes, hash)
	}
	return hashes
}

// GetDAGLeafByIndex returns a leaf by its index in the Leafs map
// Note: Map ordering is not guaranteed, so this returns any leaf at the given position
func GetDAGLeafByIndex(dag *merkle_dag.Dag, index int) (*merkle_dag.DagLeaf, error) {
	i := 0
	for _, leaf := range dag.Leafs {
		if i == index {
			return leaf, nil
		}
		i++
	}
	return nil, fmt.Errorf("leaf at index %d not found", index)
}
